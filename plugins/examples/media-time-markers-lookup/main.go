// Media Time Markers Lookup Plugin - crowdsourced skip-marker lookup by AcoustID fingerprint.
//
// This plugin implements the MediaMarkerProvider capability. For each track it's asked about,
// it: fingerprints the file (host Fingerprint service, since WASM plugins can't shell out to
// fpcalc themselves), resolves that fingerprint to an AcoustID UUID via AcoustID's public
// lookup API, then fetches that UUID's marker file from a GitHub-hosted marker repository
// (default: github.com/ThomasRedstone/media-time-markers) via jsDelivr's CDN mirror — a plain
// HTTP GET, no bulk index sync in this first version (see README for that tradeoff).
//
// A 404 from either AcoustID (no match) or the marker repo (no markers for this recording) is
// not an error: it just means no markers this run.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/mediaMarkerProvider"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
)

const (
	acoustidAPIKeyKey = "acoustidApiKey"
	repoOwnerKey      = "repoOwner"
	repoNameKey       = "repoName"
	repoRefKey        = "repoRef"

	defaultRepoOwner = "ThomasRedstone"
	defaultRepoName  = "media-time-markers"
	defaultRepoRef   = "main"

	acoustidLookupURL = "https://api.acoustid.org/v2/lookup"
	httpTimeoutMs     = 10000

	sourcePrefix = "crowdsourced:"
)

type mtmLookupPlugin struct{}

func init() {
	mediaMarkerProvider.Register(&mtmLookupPlugin{})
}

var _ mediaMarkerProvider.MediaMarkerProvider = (*mtmLookupPlugin)(nil)

func (p *mtmLookupPlugin) GetMediaMarkers(req mediaMarkerProvider.GetMediaMarkersRequest) (mediaMarkerProvider.GetMediaMarkersResponse, error) {
	track := req.Track
	if track.Path == "" {
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	apiKey, ok := pdk.GetConfig(acoustidAPIKeyKey)
	if !ok || apiKey == "" {
		pdk.Log(pdk.LogWarn, "media-time-markers-lookup: no acoustidApiKey configured, skipping")
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	fp, err := host.FingerprintCompute(host.FingerprintRequest{LibraryID: track.LibraryID, Path: track.Path})
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("media-time-markers-lookup: fingerprinting %q failed: %v", track.Path, err))
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	uuid, err := lookupAcoustID(apiKey, fp.Fingerprint, fp.DurationMs)
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("media-time-markers-lookup: AcoustID lookup for %q failed: %v", track.Path, err))
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}
	if uuid == "" {
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	file, found, err := fetchMarkerFile(uuid)
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("media-time-markers-lookup: fetching markers for %s failed: %v", uuid, err))
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}
	if !found {
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	markers := make([]mediaMarkerProvider.MediaMarkerInfo, 0, len(file.Markers))
	for _, m := range file.Markers {
		info := mediaMarkerProvider.MediaMarkerInfo{
			Kind:    m.Kind,
			StartMs: m.StartMs,
			EndMs:   m.EndMs,
		}
		if m.Confidence != nil {
			info.Confidence = float32(*m.Confidence)
		}
		markers = append(markers, info)
	}
	return mediaMarkerProvider.GetMediaMarkersResponse{Markers: markers}, nil
}

// acoustidLookupResponse is the subset of AcoustID's /v2/lookup response this plugin needs.
type acoustidLookupResponse struct {
	Status  string `json:"status"`
	Results []struct {
		ID    string  `json:"id"`
		Score float64 `json:"score"`
	} `json:"results"`
}

// lookupAcoustID resolves a Chromaprint fingerprint to an AcoustID UUID, picking the
// highest-scoring result. Returns "" (no error) when AcoustID has no match.
func lookupAcoustID(apiKey, fingerprint string, durationMs int64) (string, error) {
	durationSec := int64(math.Round(float64(durationMs) / 1000))
	reqURL := fmt.Sprintf("%s?client=%s&meta=recordingids&fingerprint=%s&duration=%d",
		acoustidLookupURL, url.QueryEscape(apiKey), url.QueryEscape(fingerprint), durationSec)

	resp, err := host.HTTPSend(host.HTTPRequest{Method: "GET", URL: reqURL, TimeoutMs: httpTimeoutMs})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("acoustid returned status %d", resp.StatusCode)
	}

	var parsed acoustidLookupResponse
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return "", fmt.Errorf("parsing acoustid response: %w", err)
	}
	if parsed.Status != "ok" || len(parsed.Results) == 0 {
		return "", nil
	}

	best := parsed.Results[0]
	for _, r := range parsed.Results[1:] {
		if r.Score > best.Score {
			best = r
		}
	}
	return best.ID, nil
}

// markerFile mirrors media-time-markers' schema/marker.schema.json.
type markerFile struct {
	AcoustID      string   `json:"acoustid"`
	DurationMs    int64    `json:"duration_ms"`
	SchemaVersion int      `json:"schema_version"`
	Markers       []marker `json:"markers"`
}

type marker struct {
	Kind       string   `json:"kind"`
	StartMs    int64    `json:"start_ms"`
	EndMs      int64    `json:"end_ms,omitempty"`
	Label      string   `json:"label,omitempty"`
	Source     string   `json:"source,omitempty"`
	Votes      int      `json:"votes,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// fetchMarkerFile fetches data/<uuid[0:2]>/<uuid[2:4]>/<uuid>.json from the configured
// marker repository via jsDelivr's GitHub CDN mirror. found is false (no error) on a 404 —
// most tracks simply won't be covered yet.
func fetchMarkerFile(uuid string) (*markerFile, bool, error) {
	owner := configOr(repoOwnerKey, defaultRepoOwner)
	repo := configOr(repoNameKey, defaultRepoName)
	ref := configOr(repoRefKey, defaultRepoRef)

	if len(uuid) < 4 {
		return nil, false, fmt.Errorf("invalid acoustid uuid: %q", uuid)
	}
	shard := uuid[0:2] + "/" + uuid[2:4]
	reqURL := fmt.Sprintf("https://cdn.jsdelivr.net/gh/%s/%s@%s/data/%s/%s.json", owner, repo, ref, shard, uuid)

	resp, err := host.HTTPSend(host.HTTPRequest{Method: "GET", URL: reqURL, TimeoutMs: httpTimeoutMs})
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode == 404 {
		return nil, false, nil
	}
	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("marker repo returned status %d", resp.StatusCode)
	}

	var file markerFile
	if err := json.Unmarshal(resp.Body, &file); err != nil {
		return nil, false, fmt.Errorf("parsing marker file: %w", err)
	}
	return &file, true, nil
}

func configOr(key, def string) string {
	v, ok := pdk.GetConfig(key)
	if !ok || v == "" {
		return def
	}
	return v
}

func main() {}
