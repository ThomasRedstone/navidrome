package host

import "context"

// SilenceSpan is a detected span of silence within a track, in milliseconds.
type SilenceSpan struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}

// SilenceDetectRequest identifies the library-relative file to analyze and the ffmpeg
// silencedetect thresholds to use.
type SilenceDetectRequest struct {
	// LibraryID is the ID of the library the file belongs to.
	LibraryID int32 `json:"libraryId"`
	// Path is the file path relative to the library root — the same shape as
	// TrackInfo.Path/LibraryID from the MediaMarkerProvider/MetadataAgent capabilities.
	Path string `json:"path"`
	// NoiseDB is the silencedetect "noise" threshold in dB (e.g. -30). Zero uses the
	// host's default.
	NoiseDB float32 `json:"noiseDb,omitempty"`
	// DurationMs is the minimum silence duration to report, in milliseconds. Zero uses
	// the host's default.
	DurationMs int32 `json:"durationMs,omitempty"`
}

// SilenceDetectResponse contains the spans of silence ffmpeg found in the file.
type SilenceDetectResponse struct {
	Spans []SilenceSpan `json:"spans"`
}

// SilenceDetectService runs ffmpeg's silencedetect filter on a library file on behalf of a
// plugin. WASM plugins have no subprocess/exec capability, so this is the server-side
// equivalent of a plugin shelling out to `ffmpeg -af silencedetect` itself.
//
// This service requires the `library` permission with `filesystem: true` (enforced by
// Manifest.Validate, not just this annotation) — the plugin supplies a library ID and a path
// relative to that library's root, never a host filesystem path, and the host resolves and
// jails it the same way library filesystem mounts are jailed elsewhere.
//
//nd:hostservice name=SilenceDetect permission=silencedetect
type SilenceDetectService interface {
	// Detect runs ffmpeg silencedetect on the given library-relative file and returns the
	// detected silence spans.
	//nd:hostfunc
	Detect(ctx context.Context, request SilenceDetectRequest) (*SilenceDetectResponse, error)
}
