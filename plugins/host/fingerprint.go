package host

import "context"

// FingerprintRequest identifies the library-relative file to fingerprint.
type FingerprintRequest struct {
	// LibraryID is the ID of the library the file belongs to.
	LibraryID int32 `json:"libraryId"`
	// Path is the file path relative to the library root — the same shape as
	// TrackInfo.Path/LibraryID.
	Path string `json:"path"`
}

// FingerprintResponse contains the computed Chromaprint fingerprint.
type FingerprintResponse struct {
	// Fingerprint is the Chromaprint fingerprint, base64-encoded as fpcalc produces it —
	// ready to send to AcoustID's lookup API as the `fingerprint` parameter.
	Fingerprint string `json:"fingerprint"`
	// DurationMs is the track duration fpcalc measured, in milliseconds — AcoustID's lookup
	// API wants this alongside the fingerprint (as whole seconds).
	DurationMs int64 `json:"durationMs"`
}

// FingerprintService computes an audio fingerprint for a library file on behalf of a plugin,
// via fpcalc (the Chromaprint/AcoustID project's CLI tool). WASM plugins have no subprocess/exec
// capability, so this is the server-side equivalent of a plugin shelling out to fpcalc itself —
// same shape as SilenceDetectService for ffmpeg.
//
// This service requires the `library` permission with `filesystem: true` (enforced by
// Manifest.Validate) — the plugin supplies a library ID and a path relative to that library's
// root, never a host filesystem path.
//
//nd:hostservice name=Fingerprint permission=fingerprint
type FingerprintService interface {
	// Compute runs fpcalc on the given library-relative file and returns its fingerprint.
	//nd:hostfunc
	Compute(ctx context.Context, request FingerprintRequest) (*FingerprintResponse, error)
}
