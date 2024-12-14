package tagging

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTagging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tagging Suite")
}
