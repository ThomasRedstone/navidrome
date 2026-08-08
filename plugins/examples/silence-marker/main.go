// Silence Marker Plugin - local lead/trail silence detection, no network dependency.
//
// This plugin implements the MediaMarkerProvider capability. For each track it's asked
// about, it calls the host SilenceDetect service (which runs ffmpeg's silencedetect filter
// server-side — WASM plugins have no subprocess/exec capability of their own) and turns any
// silence at the very start or very end of the track into skip/lead_silence /
// skip/trail_silence markers. Silence found in the middle of a track is deliberately ignored:
// this plugin answers "silence around a hidden track" (navidrome/navidrome#2082), not "flag
// every quiet passage."
package main

import (
	"fmt"
	"strconv"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/mediaMarkerProvider"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
)

const (
	noiseDBKey      = "noiseDb"
	minSilenceMsKey = "minSilenceMs"

	defaultNoiseDB      = -30
	defaultMinSilenceMs = 500

	// leadTrailToleranceMs bounds how close a silence span must sit to the track's start/end
	// to count as lead/trail silence, rather than an internal pause that happens to be near an
	// edge. ffmpeg's silencedetect timestamps are precise, so this only needs to absorb small
	// encoder padding, not measurement error.
	leadTrailToleranceMs = 250

	kindLeadSilence  = "skip/lead_silence"
	kindTrailSilence = "skip/trail_silence"
)

type silenceMarkerPlugin struct{}

func init() {
	mediaMarkerProvider.Register(&silenceMarkerPlugin{})
}

var _ mediaMarkerProvider.MediaMarkerProvider = (*silenceMarkerPlugin)(nil)

func (p *silenceMarkerPlugin) GetMediaMarkers(req mediaMarkerProvider.GetMediaMarkersRequest) (mediaMarkerProvider.GetMediaMarkersResponse, error) {
	track := req.Track
	if track.Path == "" {
		// No filesystem permission granted for this track's library, or the track has no
		// on-disk path — nothing this plugin can do.
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	resp, err := host.SilenceDetectDetect(host.SilenceDetectRequest{
		LibraryID:  track.LibraryID,
		Path:       track.Path,
		NoiseDB:    float32(getNoiseDB()),
		DurationMs: int32(getMinSilenceMs()),
	})
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("silencedetect failed for %q: %v", track.Path, err))
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}
	if len(resp.Spans) == 0 {
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	durationMs := int64(track.Duration * 1000)
	var markers []mediaMarkerProvider.MediaMarkerInfo

	first := resp.Spans[0]
	if first.StartMs <= leadTrailToleranceMs {
		markers = append(markers, mediaMarkerProvider.MediaMarkerInfo{
			Kind: kindLeadSilence, StartMs: 0, EndMs: first.EndMs,
		})
	}

	last := resp.Spans[len(resp.Spans)-1]
	if durationMs > 0 && last.EndMs >= durationMs-leadTrailToleranceMs && last.StartMs != first.StartMs {
		markers = append(markers, mediaMarkerProvider.MediaMarkerInfo{
			Kind: kindTrailSilence, StartMs: last.StartMs, EndMs: durationMs,
		})
	}

	return mediaMarkerProvider.GetMediaMarkersResponse{Markers: markers}, nil
}

func getNoiseDB() float64 {
	v, ok := pdk.GetConfig(noiseDBKey)
	if !ok || v == "" {
		return defaultNoiseDB
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultNoiseDB
	}
	return f
}

func getMinSilenceMs() int {
	v, ok := pdk.GetConfig(minSilenceMsKey)
	if !ok || v == "" {
		return defaultMinSilenceMs
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultMinSilenceMs
	}
	return n
}

func main() {}
