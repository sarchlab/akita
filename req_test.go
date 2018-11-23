package akita

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
		res, _ := ReqEquivalent(req, req)
		Expect(res).To(BeTrue())
	})

	It("should be false if different type", func() {
		r1 := NewReqBase()
		r2 := new(mockReq)
		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeFalse())
	})

	It("should match source", func() {
		r1 := NewReqBase()
		r2 := NewReqBase()
		p1 := NewLimitNumReqPort(nil, 1)
		p2 := NewLimitNumReqPort(nil, 1)

		r1.SetSrc(p1)
		r2.SetSrc(p2)
		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeFalse())
	})

	It("should match destination", func() {
		r1 := NewReqBase()
		r2 := NewReqBase()
		p1 := NewLimitNumReqPort(nil, 1)
		p2 := NewLimitNumReqPort(nil, 1)

		r1.SetDst(p1)
		r2.SetDst(p2)
		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeFalse())
	})

	It("should match other field", func() {
		r1 := new(mockReq)
		r2 := new(mockReq)

		r1.Data = 1
		r2.Data = 2

		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeFalse())
	})

	It("should match", func() {
		r1 := new(mockReq)
		r2 := new(mockReq)

		r1.Data = 1
		r2.Data = 1

		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeTrue())
	})

	It("should match nested field", func() {
		r1 := new(mockReqAdv)
		r2 := new(mockReqAdv)
		p1 := NewLimitNumReqPort(nil, 1)
		p2 := NewLimitNumReqPort(nil, 1)

		r1.SetSrc(p1)
		r2.SetSrc(p2)

		r1.Data2 = 1
		r2.Data2 = 1

		res, _ := ReqEquivalent(r1, r2)
		Expect(res).To(BeFalse())
	})

})
