package directconnection_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDirectconnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Directconnection Suite")
}
