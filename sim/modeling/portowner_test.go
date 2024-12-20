package modeling

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Port Owner", func() {
	var (
		mockCtrl *gomock.Controller
		sim      *MockSimulation
		po       PortOwnerBase
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()
		po = MakePortOwnerBase()
	})

	It("should panic if the same name is added twice", func() {
		port1 := PortBuilder{}.
			WithSimulation(sim).
			WithIncomingBufCap(10).
			WithOutgoingBufCap(10).
			Build("Port1").(*defaultPort)
		port2 := PortBuilder{}.
			WithSimulation(sim).
			WithIncomingBufCap(10).
			WithOutgoingBufCap(10).
			Build("Port2").(*defaultPort)

		po.AddPort("LocalPort", port1)
		Expect(func() { po.AddPort("LocalPort", port2) }).To(Panic())

	})

	It("should add and get port", func() {
		port := PortBuilder{}.
			WithSimulation(sim).
			WithIncomingBufCap(10).
			WithOutgoingBufCap(10).
			Build("PortA").(*defaultPort)

		po.AddPort("LocalPort", port)

		Expect(po.GetPortByName("LocalPort")).To(BeIdenticalTo(port))
	})
})
