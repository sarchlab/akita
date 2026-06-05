package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
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
			StartTask(domain, TaskStart{ParentID: 123, Kind: "kind", What: "what"})
		}).Should(Panic())
	})

	It("should be panic if domain is nil.", func() {
		Expect(func() {
			StartTask(nil, TaskStart{ID: 1, ParentID: 123, Kind: "kind", What: "what"})
		}).Should(Panic())
	})

	It("should be panic if domain's name is empty.", func() {
		domain.EXPECT().Name().Return("").AnyTimes()
		Expect(func() {
			StartTask(domain, TaskStart{ID: 1, ParentID: 123, Kind: "kind", What: "what"})
		}).Should(Panic())
	})

	It("should be panic if kind is empty.", func() {
		domain.EXPECT().Name().Return("domain").AnyTimes()
		Expect(func() {
			StartTask(domain, TaskStart{ID: 1, ParentID: 123, What: "what"})
		}).Should(Panic())
	})

	It("should be panic if what is empty.", func() {
		domain.EXPECT().Name().Return("domain").AnyTimes()
		Expect(func() {
			StartTask(domain, TaskStart{ID: 1, ParentID: 123, Kind: "kind"})
		}).Should(Panic())
	})
})
