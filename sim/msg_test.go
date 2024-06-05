package sim

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("msg", func() {
	var (
		// controlMsg *ControlMsg
		mockController *gomock.Controller
	)

	BeforeEach(func() {
		// controlMsg = ControlMsgBuilder{}
		mockController = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		mockController.Finish()
	})

	FIt("should return clone message", func() {
		controlMsg := ControlMsgBuilder{}.
			WithSrc(NewMockPort(mockController)).
			WithDst(NewMockPort(mockController)).
			WithReset().
			Build()

		cloneMsg := controlMsg.Clone()

		Expect(cloneMsg.Meta().Src).To(BeIdenticalTo(controlMsg.Meta().Src))
		Expect(cloneMsg.Meta().Dst).To(BeIdenticalTo(controlMsg.Meta().Dst))
	})
})
