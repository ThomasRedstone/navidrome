package plugins

import (
	"context"

	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SilenceDetectService", func() {
	var (
		ctx context.Context
		ds  model.DataStore
		ff  *tests.MockFFmpeg
	)

	BeforeEach(func() {
		ctx = context.Background()
		ds = &tests.MockDataStore{}
		ff = tests.NewMockFFmpeg("")

		mockLibRepo := ds.Library(ctx).(*tests.MockLibraryRepo)
		mockLibRepo.SetData(model.Libraries{{ID: 1, Name: "Music", Path: "/music"}})
	})

	It("rejects the call without filesystem permission", func() {
		service := newSilenceDetectService(ds, ff, false, nil, true)
		_, err := service.Detect(ctx, host.SilenceDetectRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a library the plugin isn't scoped to", func() {
		service := newSilenceDetectService(ds, ff, true, []int{2}, false)
		_, err := service.Detect(ctx, host.SilenceDetectRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a path that escapes the library root", func() {
		service := newSilenceDetectService(ds, ff, true, nil, true)
		_, err := service.Detect(ctx, host.SilenceDetectRequest{LibraryID: 1, Path: "../../etc/passwd"})
		Expect(err).To(HaveOccurred())
		Expect(ff.LastDetectSilencePath).To(BeEmpty(), "ffmpeg must never be invoked with an escaping path")
	})

	It("rejects an unknown library", func() {
		service := newSilenceDetectService(ds, ff, true, nil, true)
		_, err := service.Detect(ctx, host.SilenceDetectRequest{LibraryID: 999, Path: "track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("resolves the path against the library root and returns ffmpeg's spans", func() {
		ff.SilenceSpans = []ffmpeg.SilenceSpan{{StartMs: 0, EndMs: 500}, {StartMs: 190200, EndMs: 192041}}
		service := newSilenceDetectService(ds, ff, true, nil, true)

		resp, err := service.Detect(ctx, host.SilenceDetectRequest{
			LibraryID: 1, Path: "artist/album/track.mp3", NoiseDB: -35, DurationMs: 300,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(ff.LastDetectSilencePath).To(Equal("/music/artist/album/track.mp3"))
		Expect(ff.LastDetectSilenceNoise).To(Equal(-35.0))
		Expect(ff.LastDetectSilenceMinMs).To(Equal(0.3))
		Expect(resp.Spans).To(Equal([]host.SilenceSpan{
			{StartMs: 0, EndMs: 500},
			{StartMs: 190200, EndMs: 192041},
		}))
	})
})
