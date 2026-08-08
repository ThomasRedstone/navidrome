package persistence

import (
	"context"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MediaMarkerRepository", func() {
	var repo model.MediaMarkerRepository
	var marker model.MediaMarker

	BeforeEach(func() {
		ctx := log.NewContext(context.TODO())
		ctx = request.WithUser(ctx, model.User{ID: "userid", UserName: "userid", IsAdmin: true})
		repo = NewMediaMarkerRepository(ctx, GetDBXBuilder())
		marker = model.MediaMarker{
			ItemID:  "1004",
			Kind:    "skip/lead_silence",
			StartMs: 0,
			Source:  model.MediaMarkerSourceManual,
		}
		Expect(repo.Put(&marker)).To(Succeed())
	})

	AfterEach(func() {
		all, _ := repo.GetAll()
		for _, m := range all {
			_ = repo.Delete(m.ID)
		}
	})

	Describe("Put", func() {
		It("assigns an ID and item_type on create", func() {
			Expect(marker.ID).ToNot(BeEmpty())
			Expect(marker.ItemType).To(Equal(model.MediaMarkerItemTypeMediaFile))
		})

		It("updates an existing marker in place", func() {
			end := int64(500)
			marker.EndMs = &end
			Expect(repo.Put(&marker)).To(Succeed())

			res, err := repo.Get(marker.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(*res.EndMs).To(Equal(end))
		})
	})

	Describe("Get", func() {
		It("returns an existing marker", func() {
			res, err := repo.Get(marker.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.ID).To(Equal(marker.ID))
			Expect(res.Kind).To(Equal("skip/lead_silence"))
		})

		It("errors when missing", func() {
			_, err := repo.Get("notanid")
			Expect(err).To(MatchError(model.ErrNotFound))
		})
	})

	Describe("GetByItemID", func() {
		It("returns only markers for the given item, ordered by start_ms", func() {
			later := model.MediaMarker{ItemID: "1004", Kind: "skip/trail_silence", StartMs: 1000, Source: model.MediaMarkerSourceManual}
			Expect(repo.Put(&later)).To(Succeed())
			other := model.MediaMarker{ItemID: "9999", Kind: "skip/lead_silence", StartMs: 0, Source: model.MediaMarkerSourceManual}
			Expect(repo.Put(&other)).To(Succeed())

			res, err := repo.GetByItemID("1004")
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(2))
			Expect(res[0].ID).To(Equal(marker.ID))
			Expect(res[1].ID).To(Equal(later.ID))
		})

		It("returns empty for an item with no markers", func() {
			res, err := repo.GetByItemID("nonexistent")
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(BeEmpty())
		})
	})

	Describe("Delete", func() {
		It("deletes an existing marker", func() {
			Expect(repo.Delete(marker.ID)).To(Succeed())
			_, err := repo.Get(marker.ID)
			Expect(err).To(MatchError(model.ErrNotFound))
		})
	})
})
