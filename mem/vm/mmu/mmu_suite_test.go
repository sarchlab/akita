package mmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MMU Suite")
}
