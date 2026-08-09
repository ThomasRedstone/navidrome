//go:build !windows

package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/core/mediamarkers"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This is Phase 3a's real-file verification: the actual compiled silence-marker.ndp example
// plugin (plugins/examples/silence-marker), loaded through the real Manager, calling the real
// SilenceDetectService host function, which shells out to a real ffmpeg — against a real MP3
// with genuine lead and trail silence (plugins/testdata/fixtures/silence_lead_trail.mp3: 2s
// silence, 3s tone, 1.5s silence). Skipped when ffmpeg isn't installed.
var _ = Describe("Silence Marker plugin (real ffmpeg, real audio file)", Ordered, func() {
	var (
		manager *Manager
		tmpDir  string
	)

	BeforeAll(func() {
		if !ffmpeg.New().IsAvailable() {
			Skip("FFmpeg not available on this system")
		}

		var err error
		tmpDir, err = os.MkdirTemp("", "silence-marker-integration-*")
		Expect(err).ToNot(HaveOccurred())

		libraryDir := filepath.Join(tmpDir, "music-library")
		Expect(os.MkdirAll(libraryDir, 0755)).To(Succeed())

		fixture, err := os.ReadFile(filepath.Join(testdataDir, "fixtures", "silence_lead_trail.mp3"))
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(libraryDir, "hidden-track.mp3"), fixture, 0600)).To(Succeed())

		// Build the example plugin's .ndp the same way `make -C plugins/examples silence-marker`
		// does, so this test always exercises the current source rather than a stale binary.
		ndpPath := filepath.Join(tmpDir, "silence-marker"+PackageExtension)
		buildSilenceMarkerNdp(ndpPath)
		data, err := os.ReadFile(ndpPath)
		Expect(err).ToNot(HaveOccurred())
		hash := sha256.Sum256(data)

		DeferCleanup(configtest.SetupConfig())
		conf.Server.Plugins.Enabled = true
		conf.Server.Plugins.Folder = conf.NewDir(tmpDir)
		conf.Server.Plugins.AutoReload = false

		mockPluginRepo := tests.CreateMockPluginRepo()
		mockPluginRepo.Permitted = true
		mockPluginRepo.SetData(model.Plugins{{
			ID:           "silence-marker",
			Path:         ndpPath,
			SHA256:       hex.EncodeToString(hash[:]),
			Enabled:      true,
			AllLibraries: true,
		}})

		mockLibraryRepo := &tests.MockLibraryRepo{}
		mockLibraryRepo.SetData(model.Libraries{{ID: 1, Name: "Test Library", Path: libraryDir}})

		manager = &Manager{
			plugins:        make(map[string]*plugin),
			ds:             &tests.MockDataStore{MockedPlugin: mockPluginRepo, MockedLibrary: mockLibraryRepo},
			subsonicRouter: http.NotFoundHandler(),
			metrics:        noopMetricsRecorder{},
		}
		Expect(manager.Start(GinkgoT().Context())).To(Succeed())

		DeferCleanup(func() {
			_ = manager.Stop()
			_ = os.RemoveAll(tmpDir)
		})
	})

	It("reports lead and trail silence markers for the real file", func() {
		provider, ok := manager.LoadMediaMarkerProvider("silence-marker")
		Expect(ok).To(BeTrue())

		svc := mediamarkers.New(pluginLoaderFunc{
			names: func(string) []string { return []string{"silence-marker"} },
			load:  func(string) (mediamarkers.Provider, bool) { return provider, true },
		})

		mf := &model.MediaFile{
			ID: "hidden-1", Path: "hidden-track.mp3", LibraryID: 1, Duration: 6.5,
		}
		markers, err := svc.GetMarkers(context.Background(), mf)
		Expect(err).ToNot(HaveOccurred())
		Expect(markers).To(HaveLen(2))

		var lead, trail *model.MediaMarker
		for i := range markers {
			switch markers[i].Kind {
			case "skip/lead_silence":
				lead = &markers[i]
			case "skip/trail_silence":
				trail = &markers[i]
			}
		}
		Expect(lead).ToNot(BeNil())
		Expect(trail).ToNot(BeNil())

		Expect(lead.StartMs).To(Equal(int64(0)))
		Expect(*lead.EndMs).To(BeNumerically("~", 2000, 300))

		Expect(*trail.EndMs).To(BeNumerically("~", 6500, 300))
		Expect(trail.StartMs).To(BeNumerically("~", 5000, 300))

		Expect(lead.Source).To(Equal("plugin:silence-marker"))
	})
})

// pluginLoaderFunc adapts two closures to mediamarkers.PluginLoader for this test, without
// needing a full fake implementation.
type pluginLoaderFunc struct {
	names func(string) []string
	load  func(string) (mediamarkers.Provider, bool)
}

func (f pluginLoaderFunc) PluginNames(capability string) []string { return f.names(capability) }
func (f pluginLoaderFunc) LoadMediaMarkerProvider(name string) (mediamarkers.Provider, bool) {
	return f.load(name)
}

// buildSilenceMarkerNdp compiles plugins/examples/silence-marker to WASM (mainline Go's wasip1
// target, no TinyGo dependency needed — see plugins/examples/Makefile) and packages it as an
// .ndp at destPath, the same way `make -C plugins/examples silence-marker` does.
func buildSilenceMarkerNdp(destPath string) {
	_, currentFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	pluginDir := filepath.Join(filepath.Dir(currentFile), "examples", "silence-marker")

	wasmPath := filepath.Join(filepath.Dir(destPath), "silence-marker.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", wasmPath, ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "building silence-marker.wasm: %s", out)

	manifestData, err := os.ReadFile(filepath.Join(pluginDir, "manifest.json"))
	Expect(err).ToNot(HaveOccurred())
	wasmData, err := os.ReadFile(wasmPath)
	Expect(err).ToNot(HaveOccurred())

	ndpFile, err := os.Create(destPath) // #nosec G304 -- test-only, path built from t.TempDir()
	Expect(err).ToNot(HaveOccurred())
	defer ndpFile.Close()
	zw := zip.NewWriter(ndpFile)
	writeZipEntry(zw, "manifest.json", manifestData)
	writeZipEntry(zw, "plugin.wasm", wasmData)
	Expect(zw.Close()).To(Succeed())
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) {
	w, err := zw.Create(name)
	Expect(err).ToNot(HaveOccurred())
	_, err = io.Copy(w, bytes.NewReader(data))
	Expect(err).ToNot(HaveOccurred())
}
