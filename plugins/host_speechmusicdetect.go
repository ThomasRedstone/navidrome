package plugins

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/navidrome/navidrome/core/inaspeech"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
)

type speechMusicDetectServiceImpl struct {
	ds           model.DataStore
	ina          inaspeech.InaSpeech
	hasFSPerm    bool
	libraryIDMap map[int]struct{}
	allLibraries bool
}

func newSpeechMusicDetectService(ds model.DataStore, ina inaspeech.InaSpeech, hasFilesystemPerm bool, allowedLibraryIDs []int, allLibraries bool) host.SpeechMusicDetectService {
	libraryIDMap := make(map[int]struct{}, len(allowedLibraryIDs))
	for _, id := range allowedLibraryIDs {
		libraryIDMap[id] = struct{}{}
	}
	return &speechMusicDetectServiceImpl{
		ds:           ds,
		ina:          ina,
		hasFSPerm:    hasFilesystemPerm,
		libraryIDMap: libraryIDMap,
		allLibraries: allLibraries,
	}
}

func (s *speechMusicDetectServiceImpl) isLibraryAccessible(id int) bool {
	if s.allLibraries {
		return true
	}
	_, ok := s.libraryIDMap[id]
	return ok
}

// Detect resolves the request's library-relative path against the library's real root and runs
// inaSpeechSegmenter against it. The plugin never supplies (or sees) a host filesystem path —
// only a library ID plus a path relative to that library's root, mirroring SilenceDetect.
func (s *speechMusicDetectServiceImpl) Detect(ctx context.Context, request host.SpeechMusicDetectRequest) (*host.SpeechMusicDetectResponse, error) {
	if !s.hasFSPerm {
		return nil, fmt.Errorf("speechmusicdetect: library filesystem permission not granted")
	}
	libID := int(request.LibraryID)
	if !s.isLibraryAccessible(libID) {
		return nil, fmt.Errorf("speechmusicdetect: library not accessible: library ID %d is not in the allowed list", libID)
	}
	// Reject any path that could escape the library root, the same predicate the WASM
	// filesystem jail uses (sandbox_fs.go's escapes()) — the plugin never gets a raw host
	// path, so this is the only thing standing between a crafted "path" and reading outside
	// the library.
	if !filepath.IsLocal(request.Path) {
		return nil, fmt.Errorf("speechmusicdetect: invalid path")
	}

	lib, err := s.ds.Library(ctx).Get(libID)
	if err != nil {
		return nil, fmt.Errorf("speechmusicdetect: library not found: %w", err)
	}
	fullPath := filepath.Join(lib.Path, request.Path)

	segments, err := s.ina.Segment(ctx, fullPath)
	if err != nil {
		return nil, fmt.Errorf("speechmusicdetect: %w", err)
	}

	resp := &host.SpeechMusicDetectResponse{Segments: make([]host.SpeechMusicSegment, len(segments))}
	for i, seg := range segments {
		resp.Segments[i] = host.SpeechMusicSegment{Label: seg.Label, StartMs: seg.StartMs, EndMs: seg.EndMs}
	}
	return resp, nil
}

var _ host.SpeechMusicDetectService = (*speechMusicDetectServiceImpl)(nil)
