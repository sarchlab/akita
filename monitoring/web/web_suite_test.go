package web_test

import (
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/monitoring/web"
)

func TestWeb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Web Suite")
}

func firstLineMustBe(f http.File, expected string) {
	b := make([]byte, len(expected))
	_, err := f.Read(b)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(b)).To(Equal(expected))
}

var _ = Describe("Web", func() {
	Context("when in release mode", func() {
		BeforeEach(func() {
			os.Setenv("AKITA_MONITOR_DEV", "false")
		})

		It("should serve html file", func() {
			fs := web.GetAssets()

			f, err := fs.Open("index.html")

			Expect(err).ToNot(HaveOccurred())
			// First line of F must be `<!DOCTYPE html>`
			firstLineMustBe(f, "<!DOCTYPE html>")
		})
	})

	Context("when in development mode", func() {
		BeforeEach(func() {
			os.Setenv("AKITA_MONITOR_DEV", "true")
		})

		It("should serve html file", func() {
			fs := web.GetAssets()

			f, err := fs.Open("index.html")

			Expect(err).ToNot(HaveOccurred())
			// First line of F must be `<!DOCTYPE html>`
			firstLineMustBe(f, "<!DOCTYPE html>")
		})
	})
})
