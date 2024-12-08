package datamover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDatamoving(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Datamoving Suite")
}
