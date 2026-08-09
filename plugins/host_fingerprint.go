package plugins

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/navidrome/navidrome/core/chromaprint"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
)

type fingerprintServiceImpl struct {
	ds           model.DataStore
	cp           chromaprint.Chromaprint
	hasFSPerm    bool
	libraryIDMap map[int]struct{}
	allLibraries bool
}

func newFingerprintService(ds model.DataStore, cp chromaprint.Chromaprint, hasFilesystemPerm bool, allowedLibraryIDs []int, allLibraries bool) host.FingerprintService {
	libraryIDMap := make(map[int]struct{}, len(allowedLibraryIDs))
	for _, id := range allowedLibraryIDs {
		libraryIDMap[id] = struct{}{}
	}
	return &fingerprintServiceImpl{
		ds:           ds,
		cp:           cp,
		hasFSPerm:    hasFilesystemPerm,
		libraryIDMap: libraryIDMap,
		allLibraries: allLibraries,
	}
}

func (s *fingerprintServiceImpl) isLibraryAccessible(id int) bool {
	if s.allLibraries {
		return true
	}
	_, ok := s.libraryIDMap[id]
	return ok
}

// Compute resolves the request's library-relative path against the library's real root and
// runs fpcalc against it. Same path-resolution/jailing shape as silenceDetectServiceImpl.Detect.
func (s *fingerprintServiceImpl) Compute(ctx context.Context, request host.FingerprintRequest) (*host.FingerprintResponse, error) {
	if !s.hasFSPerm {
		return nil, fmt.Errorf("fingerprint: library filesystem permission not granted")
	}
	libID := int(request.LibraryID)
	if !s.isLibraryAccessible(libID) {
		return nil, fmt.Errorf("fingerprint: library not accessible: library ID %d is not in the allowed list", libID)
	}
	if !filepath.IsLocal(request.Path) {
		return nil, fmt.Errorf("fingerprint: invalid path")
	}

	lib, err := s.ds.Library(ctx).Get(libID)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: library not found: %w", err)
	}
	fullPath := filepath.Join(lib.Path, request.Path)

	result, err := s.cp.Fingerprint(ctx, fullPath)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: %w", err)
	}

	return &host.FingerprintResponse{
		Fingerprint: result.Fingerprint,
		DurationMs:  result.DurationMs,
	}, nil
}

var _ host.FingerprintService = (*fingerprintServiceImpl)(nil)
