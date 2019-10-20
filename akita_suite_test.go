package akita

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_akita_test.go" -self_package=gitlab.com/akita/akita -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Port,Engine,Event,Connection,Component,Handler,Ticker

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Akita")
}
