// Speech/Music Marker Plugin - spoken intro/outro detection via inaSpeechSegmenter.
//
// This plugin implements the MediaMarkerProvider capability. For each track it's asked
// about, it calls the host SpeechMusicDetect service (which runs inaSpeechSegmenter
// server-side — WASM plugins have no subprocess/exec capability, and inaSpeechSegmenter is a
// Python/TensorFlow tool, not something a WASM plugin could bundle itself) and turns any
// leading/trailing run of spoken content — before the music starts, or after it ends — into
// skip/intro_speech / skip/outro_speech markers.
//
// The host only ever classifies the leading/trailing WINDOW_SEC of the track (see
// core/inaspeech/segment.py), each run through Demucs vocal separation first — talk-over-a-beat
// content (a DJ/host talking over a continuous rhythmic track from second one, rather than a
// clean silence-then-speech-then-music structure) turned out to be invisible to
// inaSpeechSegmenter on the raw mix, verified against real podcast tracks. Because of that,
// this plugin processes the lead and trail windows independently rather than treating the
// combined segment list as one continuous timeline: there's a real time gap between the two
// windows for any track longer than ~2x the window size, and a vocal-isolated stem essentially
// never produces a "music" label at all (there's no music left in it to classify) — so unlike
// the original design, this can no longer rely on finding a music-labeled boundary to bound a
// talk-only track's "intro". Each window is itself already bounded (WINDOW_SEC), so a
// mis-detected intro on a genuinely talk-only episode is capped at that window's length, not
// the whole episode.
package main

import (
	"fmt"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/mediaMarkerProvider"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
)

const (
	kindIntroSpeech = "skip/intro_speech"
	kindOutroSpeech = "skip/outro_speech"
)

type speechMusicMarkerPlugin struct{}

func init() {
	mediaMarkerProvider.Register(&speechMusicMarkerPlugin{})
}

var _ mediaMarkerProvider.MediaMarkerProvider = (*speechMusicMarkerPlugin)(nil)

func (p *speechMusicMarkerPlugin) GetMediaMarkers(req mediaMarkerProvider.GetMediaMarkersRequest) (mediaMarkerProvider.GetMediaMarkersResponse, error) {
	track := req.Track
	if track.Path == "" {
		// No filesystem permission granted for this track's library, or the track has no
		// on-disk path — nothing this plugin can do.
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	resp, err := host.SpeechMusicDetectDetect(host.SpeechMusicDetectRequest{
		LibraryID: track.LibraryID,
		Path:      track.Path,
	})
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("speechmusicdetect failed for %q: %v", track.Path, err))
		return mediaMarkerProvider.GetMediaMarkersResponse{}, nil
	}

	markers := markersFromSegments(resp.Segments, int64(track.Duration*1000))
	return mediaMarkerProvider.GetMediaMarkersResponse{Markers: markers}, nil
}

func markersFromSegments(segments []host.SpeechMusicSegment, durationMs int64) []mediaMarkerProvider.MediaMarkerInfo {
	var lead, trail []host.SpeechMusicSegment
	for _, s := range segments {
		switch s.Window {
		case "lead":
			lead = append(lead, s)
		case "trail":
			trail = append(trail, s)
		}
	}

	var markers []mediaMarkerProvider.MediaMarkerInfo
	if m := introMarker(lead); m != nil {
		markers = append(markers, *m)
	}
	if m := outroMarker(trail, durationMs); m != nil {
		markers = append(markers, *m)
	}
	return markers
}

// introMarker looks at the lead window's own segments (already ordered, starting at the
// track's start) and returns a skip/intro_speech marker spanning from 0 to the end of the
// leading speech content, or nil if the window shows no speech at all.
func introMarker(lead []host.SpeechMusicSegment) *mediaMarkerProvider.MediaMarkerInfo {
	// If a music-labeled segment exists in this window, only speech before it counts — the
	// pre-Demucs behavior, kept as a fallback for whatever content still produces one.
	bound := len(lead)
	for i, s := range lead {
		if s.Label == "music" {
			bound = i
			break
		}
	}

	end := int64(-1)
	for _, s := range lead[:bound] {
		if isSpeech(s.Label) && s.EndMs > end {
			end = s.EndMs
		}
	}
	if end < 0 {
		return nil
	}
	return &mediaMarkerProvider.MediaMarkerInfo{Kind: kindIntroSpeech, StartMs: 0, EndMs: end}
}

// outroMarker looks at the trail window's own segments and returns a skip/outro_speech marker
// spanning from the start of the trailing speech content to the track's duration, or nil if the
// window shows no speech at all.
func outroMarker(trail []host.SpeechMusicSegment, durationMs int64) *mediaMarkerProvider.MediaMarkerInfo {
	// Mirrors introMarker: only speech after the last music-labeled segment counts, if one
	// exists in this window.
	bound := -1
	for i, s := range trail {
		if s.Label == "music" {
			bound = i
		}
	}

	start := int64(-1)
	for _, s := range trail[bound+1:] {
		if isSpeech(s.Label) && (start < 0 || s.StartMs < start) {
			start = s.StartMs
		}
	}
	if start < 0 {
		return nil
	}
	end := durationMs
	if len(trail) > 0 && trail[len(trail)-1].EndMs > end {
		end = trail[len(trail)-1].EndMs
	}
	return &mediaMarkerProvider.MediaMarkerInfo{Kind: kindOutroSpeech, StartMs: start, EndMs: end}
}

func isSpeech(label string) bool {
	return label == "speech" || label == "male" || label == "female"
}

func main() {}
