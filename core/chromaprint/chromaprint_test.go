package chromaprint

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestChromaprint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chromaprint Suite")
}

var _ = Describe("Chromaprint", func() {
	var cp Chromaprint

	BeforeEach(func() {
		cp = New()
		if !cp.IsAvailable() {
			Skip("fpcalc not available on this system")
		}
	})

	It("fingerprints a real audio file", func() {
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())
		fixture := filepath.Join(filepath.Dir(thisFile), "..", "..", "plugins", "testdata", "fixtures", "silence_lead_trail.mp3")

		result, err := cp.Fingerprint(GinkgoT().Context(), fixture)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Fingerprint).ToNot(BeEmpty())
		Expect(result.DurationMs).To(BeNumerically("~", 6500, 200))
	})

	It("errors for a missing file", func() {
		_, err := cp.Fingerprint(GinkgoT().Context(), "/no/such/file/really-missing.mp3")
		Expect(err).To(HaveOccurred())
	})
})
