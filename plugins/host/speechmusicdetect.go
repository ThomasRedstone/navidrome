package host

import "context"

// SpeechMusicSegment is a labeled span of a track, in milliseconds, as classified by
// inaSpeechSegmenter. Label is one of "speech", "music", "noEnergy" (silence), or "noise".
type SpeechMusicSegment struct {
	Label   string `json:"label"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
}

// SpeechMusicDetectRequest identifies the library-relative file to analyze.
type SpeechMusicDetectRequest struct {
	// LibraryID is the ID of the library the file belongs to.
	LibraryID int32 `json:"libraryId"`
	// Path is the file path relative to the library root — the same shape as
	// TrackInfo.Path/LibraryID from the MediaMarkerProvider/MetadataAgent capabilities.
	Path string `json:"path"`
}

// SpeechMusicDetectResponse contains the labeled segments inaSpeechSegmenter found in the file,
// in chronological order.
type SpeechMusicDetectResponse struct {
	Segments []SpeechMusicSegment `json:"segments"`
}

// SpeechMusicDetectService classifies a library audio file into speech/music/noise/silence
// segments on behalf of a plugin, via inaSpeechSegmenter (a Python/TensorFlow tool — see
// core/inaspeech). WASM plugins have no subprocess/exec capability and no Python runtime, so
// this is the server-side equivalent of a plugin running the classifier itself — same shape as
// SilenceDetectService for ffmpeg and FingerprintService for fpcalc.
//
// This service requires the `library` permission with `filesystem: true` (enforced by
// Manifest.Validate), and requires the server to have InaSpeechPythonPath configured — a plugin
// declaring this permission on a server without it configured gets a clear "not configured"
// error from Detect, not a missing host function.
//
//nd:hostservice name=SpeechMusicDetect permission=speechmusicdetect
type SpeechMusicDetectService interface {
	// Detect runs inaSpeechSegmenter on the given library-relative file and returns its
	// labeled segments.
	//nd:hostfunc
	Detect(ctx context.Context, request SpeechMusicDetectRequest) (*SpeechMusicDetectResponse, error)
}
