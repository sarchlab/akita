package hooking

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHooking(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hooking Suite")
}
