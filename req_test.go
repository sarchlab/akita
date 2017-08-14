package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type mockReq struct {
	ReqBase
	Data int
}

type mockReqAdv struct {
	mockReq
	Data2 int
}

var _ = Describe("ReqEquivalent", func() {

	It("should be true if the same req", func() {
		req := NewReqBase()
		Expect(ReqEquivalent(req, req)).To(BeTrue())
	})

	It("should be false if different type", func() {
		r1 := NewReqBase()
		r2 := new(mockReq)
		Expect(ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match source", func() {
		r1 := NewReqBase()
		r2 := NewReqBase()
		c1 := NewMockComponent("c1")
		c2 := NewMockComponent("c2")

		r1.SetSrc(c1)
		r2.SetSrc(c2)
		Expect(ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match destination", func() {
		r1 := NewReqBase()
		r2 := NewReqBase()
		c1 := NewMockComponent("c1")
		c2 := NewMockComponent("c2")

		r1.SetDst(c1)
		r2.SetDst(c2)
		Expect(ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match other field", func() {
		r1 := new(mockReq)
		r2 := new(mockReq)

		r1.Data = 1
		r2.Data = 2

		Expect(ReqEquivalent(r1, r2)).To(BeFalse())
	})

	It("should match", func() {
		r1 := new(mockReq)
		r2 := new(mockReq)

		r1.Data = 1
		r2.Data = 1

		Expect(ReqEquivalent(r1, r2)).To(BeTrue())
	})

	It("should match nested field", func() {
		r1 := new(mockReqAdv)
		r2 := new(mockReqAdv)
		c1 := NewMockComponent("c1")
		c2 := NewMockComponent("c2")

		r1.SetSrc(c1)
		r2.SetSrc(c2)

		r1.Data2 = 1
		r2.Data2 = 1

		Expect(ReqEquivalent(r1, r2)).To(BeFalse())
	})

})
