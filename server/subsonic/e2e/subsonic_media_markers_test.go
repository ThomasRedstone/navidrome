package e2e

import (
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Media Marker Endpoints", Ordered, func() {
	var songID string

	BeforeAll(func() {
		setupTestDB()

		songs, err := ds.MediaFile(ctx).GetAll(model.QueryOptions{Max: 1, Sort: "title"})
		Expect(err).ToNot(HaveOccurred())
		Expect(songs).ToNot(BeEmpty())
		songID = songs[0].ID
	})

	It("returns no markers for a track that has none", func() {
		resp := doReq("getMediaMarkers", "id", songID)
		Expect(resp.Status).To(Equal(responses.StatusOK))
		Expect(resp.MediaMarkers).ToNot(BeNil())
		Expect(resp.MediaMarkers.Marker).To(BeEmpty())
	})

	It("rejects a non-admin trying to create a marker", func() {
		resp := doReqWithUser(regularUser, "createMediaMarker", "id", songID, "kind", "skip/lead_silence", "startMs", "0")
		Expect(resp.Status).To(Equal(responses.StatusFailed))
		Expect(resp.Error.Code).To(Equal(int32(responses.ErrorAuthorizationFail)))
	})

	Describe("Create/Get/Update/Delete", Ordered, func() {
		var markerID string

		It("creates a manual marker", func() {
			resp := doReq("createMediaMarker", "id", songID, "kind", "skip/lead_silence", "startMs", "0", "endMs", "500")
			Expect(resp.Status).To(Equal(responses.StatusOK))
			Expect(resp.MediaMarkers.Marker).To(HaveLen(1))

			marker := resp.MediaMarkers.Marker[0]
			Expect(marker.ID).ToNot(BeEmpty())
			Expect(marker.Kind).To(Equal("skip/lead_silence"))
			Expect(marker.StartMs).To(Equal(int64(0)))
			Expect(marker.EndMs).To(Equal(int64(500)))
			Expect(marker.Source).To(Equal("manual"))
			markerID = marker.ID
		})

		It("returns the created marker via getMediaMarkers", func() {
			resp := doReq("getMediaMarkers", "id", songID)
			Expect(resp.Status).To(Equal(responses.StatusOK))
			Expect(resp.MediaMarkers.Marker).To(HaveLen(1))
			Expect(resp.MediaMarkers.Marker[0].ID).To(Equal(markerID))
		})

		It("corrects the marker", func() {
			resp := doReq("updateMediaMarker", "markerId", markerID, "kind", "skip/intro_speech", "startMs", "100", "endMs", "600")
			Expect(resp.Status).To(Equal(responses.StatusOK))
			Expect(resp.MediaMarkers.Marker).To(HaveLen(1))

			marker := resp.MediaMarkers.Marker[0]
			Expect(marker.ID).To(Equal(markerID))
			Expect(marker.Kind).To(Equal("skip/intro_speech"))
			Expect(marker.StartMs).To(Equal(int64(100)))
			Expect(marker.EndMs).To(Equal(int64(600)))
		})

		It("rejects a non-admin trying to delete a marker", func() {
			resp := doReqWithUser(regularUser, "deleteMediaMarker", "markerId", markerID)
			Expect(resp.Status).To(Equal(responses.StatusFailed))
			Expect(resp.Error.Code).To(Equal(int32(responses.ErrorAuthorizationFail)))
		})

		It("deletes the marker", func() {
			resp := doReq("deleteMediaMarker", "markerId", markerID)
			Expect(resp.Status).To(Equal(responses.StatusOK))

			resp = doReq("getMediaMarkers", "id", songID)
			Expect(resp.Status).To(Equal(responses.StatusOK))
			Expect(resp.MediaMarkers.Marker).To(BeEmpty())
		})
	})

	It("returns data-not-found for an unknown marker id on update", func() {
		resp := doReq("updateMediaMarker", "markerId", "no-such-marker", "kind", "skip/lead_silence", "startMs", "0")
		Expect(resp.Status).To(Equal(responses.StatusFailed))
		Expect(resp.Error.Code).To(Equal(int32(responses.ErrorDataNotFound)))
	})
})
