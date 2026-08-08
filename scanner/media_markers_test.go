package scanner_test

import (
	"context"
	"path/filepath"
	"testing/fstest"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/core/storage/storagetest"
	"github.com/navidrome/navidrome/db"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/persistence"
	"github.com/navidrome/navidrome/scanner"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeMediaMarkers is a stand-in for a MediaMarkerProvider plugin: it returns a fixed marker
// for every track it's asked about, so a scan-time test can assert markers actually get
// persisted without needing a real WASM plugin.
type fakeMediaMarkers struct {
	calls int
}

func (f *fakeMediaMarkers) GetMarkers(_ context.Context, mf *model.MediaFile) ([]model.MediaMarker, error) {
	f.calls++
	return []model.MediaMarker{{
		ItemID:  mf.ID,
		Kind:    "skip/lead_silence",
		StartMs: 0,
		Source:  model.MediaMarkerSourcePluginPrefix + "fake",
	}}, nil
}

var _ = Describe("Scanner: media markers", Ordered, func() {
	var ctx context.Context
	var lib model.Library
	var ds model.DataStore
	var fake *fakeMediaMarkers
	var s model.Scanner

	BeforeAll(func() {
		tests.SkipOnWindows("SQLite file lock blocks TempDir cleanup (#TBD-path-sep-scanner)")
		ctx = request.WithUser(GinkgoT().Context(), model.User{ID: "123", IsAdmin: true})
		tmpDir := GinkgoT().TempDir()
		conf.Server.DbPath = filepath.Join(tmpDir, "test-media-markers-scan.db?_journal_mode=WAL")
		log.Warn("Using DB at " + conf.Server.DbPath)
		db.Db().SetMaxOpenConns(1)
	})

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.MusicFolder = "fake:///music"
		conf.Server.DevExternalScanner = false

		db.Init(ctx)
		DeferCleanup(func() {
			Expect(tests.ClearDB()).To(Succeed())
		})

		ds = persistence.New(db.Db())

		adminUser := model.User{ID: "123", UserName: "admin", Name: "Admin User", IsAdmin: true, NewPassword: "password"}
		Expect(ds.User(ctx).Put(&adminUser)).To(Succeed())

		fake = &fakeMediaMarkers{}
		s = scanner.New(ctx, ds, artwork.NoopCacheWarmer(), events.NoopBroker(),
			playlists.NewPlaylists(ds, core.NewImageUploadService()), metrics.NewNoopInstance(), fake)

		lib = model.Library{ID: 1, Name: "Fake Library", Path: "fake:///music"}
		Expect(ds.Library(ctx).Put(&lib)).To(Succeed())

		fsys := storagetest.FakeFS{}
		storagetest.Register("fake", &fsys)
	})

	It("persists markers returned by an installed MediaMarkerProvider plugin", func() {
		album := template(_t{"albumartist": "Marker Artist", "album": "Marker Album"})
		createFS(fstest.MapFS{
			"Marker Artist/Marker Album/01 - Intro.mp3": album(track(1, "Intro")),
		})

		_, err := s.ScanAll(ctx, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(fake.calls).To(Equal(1))

		mfs, err := ds.MediaFile(ctx).GetAll()
		Expect(err).ToNot(HaveOccurred())
		Expect(mfs).To(HaveLen(1))

		markers, err := ds.MediaMarker(ctx).GetByItemID(mfs[0].ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(markers).To(HaveLen(1))
		Expect(markers[0].Kind).To(Equal("skip/lead_silence"))
		Expect(markers[0].Source).To(Equal("plugin:fake"))
	})

	It("does not accumulate duplicate markers on a re-scan of an unchanged track", func() {
		album := template(_t{"albumartist": "Marker Artist", "album": "Marker Album"})
		createFS(fstest.MapFS{
			"Marker Artist/Marker Album/01 - Intro.mp3": album(track(1, "Intro")),
		})

		_, err := s.ScanAll(ctx, true)
		Expect(err).ToNot(HaveOccurred())
		_, err = s.ScanAll(ctx, true)
		Expect(err).ToNot(HaveOccurred())

		mfs, err := ds.MediaFile(ctx).GetAll()
		Expect(err).ToNot(HaveOccurred())
		Expect(mfs).To(HaveLen(1))

		markers, err := ds.MediaMarker(ctx).GetByItemID(mfs[0].ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(markers).To(HaveLen(1))
	})
})
