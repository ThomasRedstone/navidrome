package plugins

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/core/chromaprint"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FingerprintService", func() {
	var (
		ctx context.Context
		ds  model.DataStore
		cp  *tests.MockChromaprint
	)

	BeforeEach(func() {
		ctx = context.Background()
		ds = &tests.MockDataStore{}
		cp = tests.NewMockChromaprint()

		mockLibRepo := ds.Library(ctx).(*tests.MockLibraryRepo)
		mockLibRepo.SetData(model.Libraries{{ID: 1, Name: "Music", Path: "/music"}})
	})

	It("rejects the call without filesystem permission", func() {
		service := newFingerprintService(ds, cp, false, nil, true)
		_, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a library the plugin isn't scoped to", func() {
		service := newFingerprintService(ds, cp, true, []int{2}, false)
		_, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a path that escapes the library root", func() {
		service := newFingerprintService(ds, cp, true, nil, true)
		_, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 1, Path: "../../etc/passwd"})
		Expect(err).To(HaveOccurred())
		Expect(cp.LastFingerprintPath).To(BeEmpty(), "fpcalc must never be invoked with an escaping path")
	})

	It("rejects an unknown library", func() {
		service := newFingerprintService(ds, cp, true, nil, true)
		_, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 999, Path: "track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("resolves the path against the library root and returns the fingerprint", func() {
		cp.Result = &chromaprint.FingerprintResult{Fingerprint: "AQAAH0mSJFmS", DurationMs: 6500}
		service := newFingerprintService(ds, cp, true, nil, true)

		resp, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).ToNot(HaveOccurred())
		Expect(cp.LastFingerprintPath).To(Equal("/music/artist/album/track.mp3"))
		Expect(resp.Fingerprint).To(Equal("AQAAH0mSJFmS"))
		Expect(resp.DurationMs).To(Equal(int64(6500)))
	})

	It("propagates fpcalc errors", func() {
		cp.Err = errors.New("fpcalc not found")
		service := newFingerprintService(ds, cp, true, nil, true)
		_, err := service.Compute(ctx, host.FingerprintRequest{LibraryID: 1, Path: "track.mp3"})
		Expect(err).To(HaveOccurred())
	})
})
