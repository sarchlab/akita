package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Port Owner", func() {
	var (
		po *PortOwnerBase
	)

	BeforeEach(func() {
		po = NewPortOwnerBase()
	})

	It("shoud panic if the same name is added twice", func() {
		port1 := NewLimitNumMsgPort(nil, 10, "Port1")
		port2 := NewLimitNumMsgPort(nil, 10, "Port2")

		po.AddPort("LocalPort", port1)
		Expect(func() { po.AddPort("LocalPort", port2) }).To(Panic())

	})

	It("should add and get port", func() {
		port := NewLimitNumMsgPort(nil, 10, "PortA")

		po.AddPort("LocalPort", port)

		Expect(po.GetPortByName("LocalPort")).To(BeIdenticalTo(port))
	})
})
