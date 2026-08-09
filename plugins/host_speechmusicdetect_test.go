package plugins

import (
	"context"

	"github.com/navidrome/navidrome/core/inaspeech"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SpeechMusicDetectService", func() {
	var (
		ctx context.Context
		ds  model.DataStore
		ina *tests.MockInaSpeech
	)

	BeforeEach(func() {
		ctx = context.Background()
		ds = &tests.MockDataStore{}
		ina = tests.NewMockInaSpeech()

		mockLibRepo := ds.Library(ctx).(*tests.MockLibraryRepo)
		mockLibRepo.SetData(model.Libraries{{ID: 1, Name: "Music", Path: "/music"}})
	})

	It("rejects the call without filesystem permission", func() {
		service := newSpeechMusicDetectService(ds, ina, false, nil, true)
		_, err := service.Detect(ctx, host.SpeechMusicDetectRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a library the plugin isn't scoped to", func() {
		service := newSpeechMusicDetectService(ds, ina, true, []int{2}, false)
		_, err := service.Detect(ctx, host.SpeechMusicDetectRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a path that escapes the library root", func() {
		service := newSpeechMusicDetectService(ds, ina, true, nil, true)
		_, err := service.Detect(ctx, host.SpeechMusicDetectRequest{LibraryID: 1, Path: "../../etc/passwd"})
		Expect(err).To(HaveOccurred())
		Expect(ina.LastSegmentPath).To(BeEmpty(), "inaSpeechSegmenter must never be invoked with an escaping path")
	})

	It("rejects an unknown library", func() {
		service := newSpeechMusicDetectService(ds, ina, true, nil, true)
		_, err := service.Detect(ctx, host.SpeechMusicDetectRequest{LibraryID: 999, Path: "track.mp3"})
		Expect(err).To(HaveOccurred())
	})

	It("resolves the path against the library root and returns the classified segments", func() {
		ina.Segments = []inaspeech.Segment{
			{Label: "speech", StartMs: 0, EndMs: 5000},
			{Label: "music", StartMs: 5000, EndMs: 190000},
		}
		service := newSpeechMusicDetectService(ds, ina, true, nil, true)

		resp, err := service.Detect(ctx, host.SpeechMusicDetectRequest{LibraryID: 1, Path: "artist/album/track.mp3"})
		Expect(err).ToNot(HaveOccurred())
		Expect(ina.LastSegmentPath).To(Equal("/music/artist/album/track.mp3"))
		Expect(resp.Segments).To(Equal([]host.SpeechMusicSegment{
			{Label: "speech", StartMs: 0, EndMs: 5000},
			{Label: "music", StartMs: 5000, EndMs: 190000},
		}))
	})
})
