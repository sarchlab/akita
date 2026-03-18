package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimpleBankedMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleBankedMemory Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
