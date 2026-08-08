package plugins

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
)

type silenceDetectServiceImpl struct {
	ds           model.DataStore
	ff           ffmpeg.FFmpeg
	hasFSPerm    bool
	libraryIDMap map[int]struct{}
	allLibraries bool
}

func newSilenceDetectService(ds model.DataStore, ff ffmpeg.FFmpeg, hasFilesystemPerm bool, allowedLibraryIDs []int, allLibraries bool) host.SilenceDetectService {
	libraryIDMap := make(map[int]struct{}, len(allowedLibraryIDs))
	for _, id := range allowedLibraryIDs {
		libraryIDMap[id] = struct{}{}
	}
	return &silenceDetectServiceImpl{
		ds:           ds,
		ff:           ff,
		hasFSPerm:    hasFilesystemPerm,
		libraryIDMap: libraryIDMap,
		allLibraries: allLibraries,
	}
}

func (s *silenceDetectServiceImpl) isLibraryAccessible(id int) bool {
	if s.allLibraries {
		return true
	}
	_, ok := s.libraryIDMap[id]
	return ok
}

// Detect resolves the request's library-relative path against the library's real root and
// runs ffmpeg silencedetect against it. The plugin never supplies (or sees) a host filesystem
// path — only a library ID plus a path relative to that library's root, mirroring
// TrackInfo.Path/LibraryID.
func (s *silenceDetectServiceImpl) Detect(ctx context.Context, request host.SilenceDetectRequest) (*host.SilenceDetectResponse, error) {
	if !s.hasFSPerm {
		return nil, fmt.Errorf("silencedetect: library filesystem permission not granted")
	}
	libID := int(request.LibraryID)
	if !s.isLibraryAccessible(libID) {
		return nil, fmt.Errorf("silencedetect: library not accessible: library ID %d is not in the allowed list", libID)
	}
	// Reject any path that could escape the library root, the same predicate the WASM
	// filesystem jail uses (sandbox_fs.go's escapes()) — the plugin never gets a raw host path,
	// so this is the only thing standing between a crafted "path" and reading outside the
	// library.
	if !filepath.IsLocal(request.Path) {
		return nil, fmt.Errorf("silencedetect: invalid path")
	}

	lib, err := s.ds.Library(ctx).Get(libID)
	if err != nil {
		return nil, fmt.Errorf("silencedetect: library not found: %w", err)
	}
	fullPath := filepath.Join(lib.Path, request.Path)

	noiseDB := float64(request.NoiseDB)
	minDurationSec := float64(request.DurationMs) / 1000
	spans, err := s.ff.DetectSilence(ctx, fullPath, noiseDB, minDurationSec)
	if err != nil {
		return nil, fmt.Errorf("silencedetect: %w", err)
	}

	resp := &host.SilenceDetectResponse{Spans: make([]host.SilenceSpan, len(spans))}
	for i, sp := range spans {
		resp.Spans[i] = host.SilenceSpan{StartMs: sp.StartMs, EndMs: sp.EndMs}
	}
	return resp, nil
}

var _ host.SilenceDetectService = (*silenceDetectServiceImpl)(nil)
