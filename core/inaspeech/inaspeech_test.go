package inaspeech

import (
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInaSpeech(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InaSpeech Suite")
}

var _ = Describe("InaSpeech", func() {
	var ina InaSpeech

	BeforeEach(func() {
		// Unlike ffmpeg/fpcalc, there's no PATH-discovered default — this test only runs
		// where an admin has explicitly pointed ND_INASPEECHPYTHONPATH at an interpreter
		// with inaSpeechSegmenter installed (a heavy TensorFlow-based venv, not expected in
		// a normal CI image).
		ina = New()
		if !ina.IsAvailable() {
			Skip("InaSpeechPythonPath not configured, or interpreter not found")
		}
	})

	It("segments a real audio file", func() {
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())
		fixture := filepath.Join(filepath.Dir(thisFile), "..", "..", "plugins", "testdata", "fixtures", "silence_lead_trail.mp3")

		segments, err := ina.Segment(GinkgoT().Context(), fixture)
		Expect(err).ToNot(HaveOccurred())
		Expect(segments).ToNot(BeEmpty())
		for _, s := range segments {
			Expect(s.EndMs).To(BeNumerically(">", s.StartMs))
		}
	})

	It("errors for a missing file", func() {
		_, err := ina.Segment(GinkgoT().Context(), "/no/such/file/really-missing.mp3")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("pythonCmd", func() {
	It("errors clearly when InaSpeechPythonPath is not configured", func() {
		orig := conf.Server.InaSpeechPythonPath
		defer func() { conf.Server.InaSpeechPythonPath = orig }()
		conf.Server.InaSpeechPythonPath = ""
		pythonOnce = sync.Once{}
		_, err := pythonCmd()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not configured"))
	})
})
