package cache_test

import "testing"
import "github.com/onsi/gomega"
import "github.com/onsi/ginkgo"

func TestCache(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cache System")
}
