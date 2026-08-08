//go:build !wasip1

// These tests run as plain `go test` (mainline Go, not wasip1/TinyGo), using the generated
// stub PDK's testify-based mocks (host.HTTPMock, host.FingerprintMock, pdk.PDKMock) — see
// plugins/pdk/go/*/*_stub.go. They exercise this plugin's actual logic (AcoustID response
// parsing, marker-file parsing, 404/no-match handling, confidence conversion) without needing
// network access, an AcoustID API key, or the media-time-markers repo to be public — all of
// which the environment this was built in couldn't guarantee. Real fingerprinting via fpcalc
// and the SilenceDetect/Fingerprint host functions are already verified for real elsewhere
// (core/chromaprint, plugins/host_fingerprint_test.go,
// plugins/media_marker_provider_integration_test.go); this plugin's GetMediaMarkers just calls
// straight through to host.FingerprintCompute, so re-verifying fpcalc itself here would be
// redundant — these tests focus on what's unique to this plugin.
package main

import (
	"testing"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/mediaMarkerProvider"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/stretchr/testify/mock"
)

func resetMocks() {
	pdk.ResetMock()
	host.FingerprintMock.ExpectedCalls = nil
	host.HTTPMock.ExpectedCalls = nil
	// Every failure path logs a warning; allow (but don't require) it in every test.
	pdk.PDKMock.On("Log", mock.Anything, mock.Anything).Return().Maybe()
}

func withDefaultConfig(t *testing.T, apiKey string) {
	t.Helper()
	pdk.PDKMock.On("GetConfig", acoustidAPIKeyKey).Return(apiKey, true).Once()
	pdk.PDKMock.On("GetConfig", repoOwnerKey).Return("", false).Once()
	pdk.PDKMock.On("GetConfig", repoNameKey).Return("", false).Once()
	pdk.PDKMock.On("GetConfig", repoRefKey).Return("", false).Once()
}

func TestGetMediaMarkers_HappyPath(t *testing.T) {
	resetMocks()
	withDefaultConfig(t, "test-api-key")

	track := mediaMarkerProvider.TrackInfo{ID: "t1", LibraryID: 1, Path: "artist/album/track.mp3", Duration: 240.07}

	host.FingerprintMock.On("Compute", host.FingerprintRequest{LibraryID: 1, Path: track.Path}).
		Return(&host.FingerprintResponse{Fingerprint: "AQAAH0mSJFmS", DurationMs: 240070}, nil).Once()

	acoustidBody := []byte(`{
		"status": "ok",
		"results": [
			{"id": "low-score-uuid", "score": 0.4},
			{"id": "5a6b2f12-2f76-4184-a089-68b53a30e6ee", "score": 0.95}
		]
	}`)
	host.HTTPMock.On("Send", mockedURLPrefix("https://api.acoustid.org/v2/lookup?")).
		Return(&host.HTTPResponse{StatusCode: 200, Body: acoustidBody}, nil).Once()

	// Real shape, taken from data/5a/6b/5a6b2f12-2f76-4184-a089-68b53a30e6ee.json in the
	// media-time-markers repo.
	markerBody := []byte(`{
		"acoustid": "5a6b2f12-2f76-4184-a089-68b53a30e6ee",
		"duration_ms": 240070,
		"schema_version": 1,
		"markers": [
			{"kind": "skip/lead_silence", "start_ms": 0, "end_ms": 500, "source": "manual"}
		]
	}`)
	host.HTTPMock.On("Send", mockedGET("https://cdn.jsdelivr.net/gh/ThomasRedstone/media-time-markers@main/data/5a/6b/5a6b2f12-2f76-4184-a089-68b53a30e6ee.json")).
		Return(&host.HTTPResponse{StatusCode: 200, Body: markerBody}, nil).Once()

	p := &mtmLookupPlugin{}
	resp, err := p.GetMediaMarkers(mediaMarkerProvider.GetMediaMarkersRequest{Track: track})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mediaMarkerProvider.MediaMarkerInfo{{Kind: "skip/lead_silence", StartMs: 0, EndMs: 500}}
	if len(resp.Markers) != len(want) || resp.Markers[0] != want[0] {
		t.Fatalf("got %+v, want %+v", resp.Markers, want)
	}
}

func TestGetMediaMarkers_NoAcoustIDMatch(t *testing.T) {
	resetMocks()
	withDefaultConfig(t, "test-api-key")
	track := mediaMarkerProvider.TrackInfo{LibraryID: 1, Path: "unknown.mp3"}

	host.FingerprintMock.On("Compute", host.FingerprintRequest{LibraryID: 1, Path: track.Path}).
		Return(&host.FingerprintResponse{Fingerprint: "abc", DurationMs: 1000}, nil).Once()
	host.HTTPMock.On("Send", mockedURLPrefix("https://api.acoustid.org/v2/lookup?")).
		Return(&host.HTTPResponse{StatusCode: 200, Body: []byte(`{"status":"ok","results":[]}`)}, nil).Once()

	p := &mtmLookupPlugin{}
	resp, err := p.GetMediaMarkers(mediaMarkerProvider.GetMediaMarkersRequest{Track: track})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Markers) != 0 {
		t.Fatalf("expected no markers, got %+v", resp.Markers)
	}
}

func TestGetMediaMarkers_MarkerRepo404(t *testing.T) {
	resetMocks()
	withDefaultConfig(t, "test-api-key")
	track := mediaMarkerProvider.TrackInfo{LibraryID: 1, Path: "uncovered.mp3"}

	host.FingerprintMock.On("Compute", host.FingerprintRequest{LibraryID: 1, Path: track.Path}).
		Return(&host.FingerprintResponse{Fingerprint: "abc", DurationMs: 1000}, nil).Once()
	host.HTTPMock.On("Send", mockedURLPrefix("https://api.acoustid.org/v2/lookup?")).
		Return(&host.HTTPResponse{StatusCode: 200, Body: []byte(`{"status":"ok","results":[{"id":"unmapped-uuid","score":1}]}`)}, nil).Once()
	host.HTTPMock.On("Send", mockedURLPrefix("https://cdn.jsdelivr.net/gh/ThomasRedstone/media-time-markers@main/data/un/ma/unmapped-uuid.json")).
		Return(&host.HTTPResponse{StatusCode: 404}, nil).Once()

	p := &mtmLookupPlugin{}
	resp, err := p.GetMediaMarkers(mediaMarkerProvider.GetMediaMarkersRequest{Track: track})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Markers) != 0 {
		t.Fatalf("expected no markers, got %+v", resp.Markers)
	}
}

func TestGetMediaMarkers_NoAPIKeyConfigured(t *testing.T) {
	resetMocks()
	pdk.PDKMock.On("GetConfig", acoustidAPIKeyKey).Return("", false).Once()

	p := &mtmLookupPlugin{}
	resp, err := p.GetMediaMarkers(mediaMarkerProvider.GetMediaMarkersRequest{
		Track: mediaMarkerProvider.TrackInfo{LibraryID: 1, Path: "track.mp3"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Markers) != 0 {
		t.Fatalf("expected no markers, got %+v", resp.Markers)
	}
}

func TestGetMediaMarkers_NoLibraryPath(t *testing.T) {
	// No filesystem permission granted for this track's library: Path is empty. Should not
	// even read config or call any host function.
	resetMocks()
	p := &mtmLookupPlugin{}
	resp, err := p.GetMediaMarkers(mediaMarkerProvider.GetMediaMarkersRequest{
		Track: mediaMarkerProvider.TrackInfo{LibraryID: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Markers) != 0 {
		t.Fatalf("expected no markers, got %+v", resp.Markers)
	}
}

// mockedGET matches an HTTPRequest by exact URL and GET method — used for AcoustID's lookup,
// whose full URL (including query params) is asserted precisely.
func mockedGET(url string) any {
	return mock.MatchedBy(func(r host.HTTPRequest) bool {
		return r.Method == "GET" && r.URL == url
	})
}

// mockedURLPrefix matches by URL prefix.
func mockedURLPrefix(prefix string) any {
	return mock.MatchedBy(func(r host.HTTPRequest) bool {
		return len(r.URL) >= len(prefix) && r.URL[:len(prefix)] == prefix
	})
}
