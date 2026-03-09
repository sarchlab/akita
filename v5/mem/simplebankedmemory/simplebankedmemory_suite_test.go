package simplebankedmemory

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimpleBankedMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleBankedMemory Suite")
}
