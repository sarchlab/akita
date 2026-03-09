package wiring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWiring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Wiring Suite")
}
