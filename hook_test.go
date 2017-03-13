package core_test

import (
	"reflect"

	"gitlab.com/yaotsu/core"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

type hookCall struct {
	item   interface{}
	domain core.Hookable
}

type MockHook struct {
	hookType      reflect.Type
	hookPos       core.HookPos
	expectedCalls []hookCall
}

// NewMockHook returns a new MockHook object
func NewMockHook(hookType reflect.Type, pos core.HookPos) *MockHook {
	return &MockHook{hookType, pos, make([]hookCall, 0)}
}

func (h MockHook) Type() reflect.Type {
	return h.hookType
}

func (h MockHook) Pos() core.HookPos {
	return h.hookPos
}

func (h *MockHook) ExpectHookCall(item interface{}, domain core.Hookable) {
	h.expectedCalls = append(h.expectedCalls, hookCall{item, domain})
}

func (h *MockHook) AllExpectedCalled() {
	gomega.Expect(h.expectedCalls).To(gomega.BeEmpty())
}

func (h *MockHook) Func(item interface{}, domain core.Hookable) {
	gomega.Expect(h.expectedCalls).NotTo(gomega.BeEmpty())
	gomega.Expect(h.expectedCalls[0].item).To(gomega.BeIdenticalTo(item))
	gomega.Expect(h.expectedCalls[0].domain).To(gomega.BeIdenticalTo(domain))
	h.expectedCalls = h.expectedCalls[1:]
}

type SomeType struct {
}

type SomeType2 int32

var _ = Describe("BasicHookable", func() {
	It("should allow basic hooking", func() {
		domain := core.NewBasicHookable()
		hook := NewMockHook(reflect.TypeOf((*SomeType)(nil)), core.BeforeEvent)
		domain.Accept(hook)

		item := new(SomeType)

		hook.ExpectHookCall(item, domain)

		domain.Invoke(item, core.BeforeEvent)

		hook.AllExpectedCalled()
	})

	It("should not invoke if not hooking at the exact position", func() {
		domain := core.NewBasicHookable()
		hook := NewMockHook(reflect.TypeOf((*SomeType)(nil)), core.BeforeEvent)
		domain.Accept(hook)

		item := new(SomeType)

		domain.Invoke(item, core.AfterEvent)
	})

	It("should allow any position hooking", func() {
		domain := core.NewBasicHookable()
		hook := NewMockHook(reflect.TypeOf((*SomeType)(nil)), core.Any)
		domain.Accept(hook)

		item := new(SomeType)

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item, domain)
		domain.Invoke(item, core.AfterEvent)
		domain.Invoke(item, core.BeforeEvent)
		hook.AllExpectedCalled()
	})

	It("should allow hooking on an interface", func() {
		domain := core.NewBasicHookable()
		hook := NewMockHook(reflect.TypeOf((*interface{})(nil)), core.AfterEvent)
		domain.Accept(hook)

		item := new(SomeType)
		item2 := new(SomeType2)

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item2, domain)
		domain.Invoke(item, core.AfterEvent)
		domain.Invoke(item2, core.AfterEvent)
		hook.AllExpectedCalled()
	})

	It("should allow hooking on any type", func() {
		domain := core.NewBasicHookable()
		hook := NewMockHook(nil, core.AfterEvent)
		domain.Accept(hook)

		item := new(SomeType)
		item2 := new(SomeType2)
		item3 := 12

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item2, domain)
		hook.ExpectHookCall(item3, domain)
		domain.Invoke(item, core.AfterEvent)
		domain.Invoke(item2, core.AfterEvent)
		domain.Invoke(item3, core.AfterEvent)
		hook.AllExpectedCalled()
	})

})
