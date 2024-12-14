package mshr

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMSHR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MSHR Suite")
}
