package tracing

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Api", func() {
	var (
		mockCtrl *gomock.Controller
		domain   *MockNamedHookable
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		domain = NewMockNamedHookable(mockCtrl)
		domain.EXPECT().NumHooks().Return(1).AnyTimes()
		domain.EXPECT().InvokeHook(gomock.Any()).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should panic if ID is not given", func() {
		domain.EXPECT().Name().Return("domain").AnyTimes()
		Expect(func() {
			StartTask("", "123", domain, "kind", "what", nil)
		}).Should(Panic())
	})

	It("should be panic if domain is nil.", func() {
		Expect(func() {
			StartTask("id", "123", nil, "kind", "what", nil)
		}).Should(Panic())
	})

	It("should be panic if domain's name is empty.", func() {
		domain.EXPECT().Name().Return("").AnyTimes()
		Expect(func() {
			StartTask("id", "123", domain, "kind", "what", nil)
		}).Should(Panic())
	})

	It("should be panic if kind is empty.", func() {
		domain.EXPECT().Name().Return("domain").AnyTimes()
		Expect(func() {
			StartTask("id", "123", domain, "", "what", nil)
		}).Should(Panic())
	})

	It("should be panic if what is empty.", func() {
		domain.EXPECT().Name().Return("domain").AnyTimes()
		Expect(func() {
			StartTask("id", "123", domain, "kind", "", nil)
		}).Should(Panic())
	})
})
