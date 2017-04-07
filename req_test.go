package core_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

type MockReq struct {
	core.ReqBase
	Data int
}

type MockReqAdv struct {
	MockReq
	Data2 int
}

var _ = Describe("ReqEquivalent", func() {

	It("should be true if the same req", func() {
		req := core.NewReqBase()
		Expect(core.ReqEquivalent(req, req)).To(BeTrue())
	})

	It("should be false if different type", func() {
		r1 := core.NewReqBase()
		r2 := new(MockReq)
		Expect(core.ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match source", func() {
		r1 := core.NewReqBase()
		r2 := core.NewReqBase()
		c1 := core.NewMockComponent("c1")
		c2 := core.NewMockComponent("c2")

		r1.SetSrc(c1)
		r2.SetSrc(c2)
		Expect(core.ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match destination", func() {
		r1 := core.NewReqBase()
		r2 := core.NewReqBase()
		c1 := core.NewMockComponent("c1")
		c2 := core.NewMockComponent("c2")

		r1.SetDst(c1)
		r2.SetDst(c2)
		Expect(core.ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match other field", func() {
		r1 := new(MockReq)
		r2 := new(MockReq)

		r1.Data = 1
		r2.Data = 2

		Expect(core.ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match", func() {
		r1 := new(MockReq)
		r2 := new(MockReq)

		r1.Data = 1
		r2.Data = 1

		Expect(core.ReqEquivalent(r1, r2)).To(BeTrue())
	})

	It("should match nested field", func() {
		r1 := new(MockReqAdv)
		r2 := new(MockReqAdv)
		c1 := core.NewMockComponent("c1")
		c2 := core.NewMockComponent("c2")

		r1.SetSrc(c1)
		r2.SetSrc(c2)

		r1.Data2 = 1
		r2.Data2 = 1

		Expect(core.ReqEquivalent(r1, r2)).To(BeFalse())
	})

})
