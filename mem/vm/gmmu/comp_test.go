package gmmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v4/sim"
)

var _ = Describe("Builder", func() {

	var (
		mockCtrl  *gomock.Controller
		engine    *MockEngine
		topPort   *MockPort
		pageTable *MockPageTable
		gmmu      *GMMU
		// mmuMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		builder := MakeBuilder().WithEngine(engine)
		gmmu = builder.Build("MMU")
		gmmu.topPort = topPort
		gmmu.pageTable = pageTable
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("GMMU Builder", func() {
		It("should build GMMU correctly", func() {
			Expect(gmmu.Engine).To(Equal(engine))
			Expect(gmmu.Freq).To(Equal(1 * sim.GHz))
			Expect(gmmu.maxRequestsInFlight).To(Equal(16))
			Expect(gmmu.pageTable).To(Equal(pageTable))
			Expect(gmmu.topPort).To(Equal(topPort))
		})
	})
})
