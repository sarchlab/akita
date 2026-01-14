package gmmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GMMU Suite")
}
