// Speech/Music Marker Plugin - spoken intro/outro detection via inaSpeechSegmenter.
//
// This plugin implements the MediaMarkerProvider capability. For each track it's asked
// about, it calls the host SpeechMusicDetect service (which runs inaSpeechSegmenter
// server-side — WASM plugins have no subprocess/exec capability, and inaSpeechSegmenter is a
// Python/TensorFlow tool, not something a WASM plugin could bundle itself) and turns any
// leading/trailing run of spoken content — before the music starts, or after it ends — into
// skip/intro_speech / skip/outro_speech markers.
//
// Unlike silence-marker (lead/trail silence), this plugin only emits a marker when the track
// actually contains a music segment: a track with no music segment at all (e.g. a talk-only
// podcast episode) has nothing to "skip into" or "skip out of", so it's left alone rather than
// flagging the whole thing as one giant intro.
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
	musicStart, musicEnd := -1, -1
	for i, s := range segments {
		if s.Label == "music" {
			if musicStart == -1 {
				musicStart = i
			}
			musicEnd = i
		}
	}
	// No music segment at all — nothing for an intro/outro marker to bound. This deliberately
	// leaves talk-only episodes alone rather than flagging the entire track.
	if musicStart == -1 {
		return nil
	}

	var markers []mediaMarkerProvider.MediaMarkerInfo

	if lead := segments[:musicStart]; hasSpeech(lead) {
		markers = append(markers, mediaMarkerProvider.MediaMarkerInfo{
			Kind: kindIntroSpeech, StartMs: 0, EndMs: lead[len(lead)-1].EndMs,
		})
	}

	if trail := segments[musicEnd+1:]; hasSpeech(trail) {
		end := trail[len(trail)-1].EndMs
		if durationMs > end {
			end = durationMs
		}
		markers = append(markers, mediaMarkerProvider.MediaMarkerInfo{
			Kind: kindOutroSpeech, StartMs: trail[0].StartMs, EndMs: end,
		})
	}

	return markers
}

func hasSpeech(segments []host.SpeechMusicSegment) bool {
	for _, s := range segments {
		if s.Label == "speech" || s.Label == "male" || s.Label == "female" {
			return true
		}
	}
	return false
}

func main() {}
