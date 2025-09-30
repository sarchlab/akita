package queueingv5

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestQueueingv5(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Queueingv5 Suite")
}