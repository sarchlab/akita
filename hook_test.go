package core

import (
	"reflect"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

type hookCall struct {
	item   interface{}
	domain Hookable
}

type MockHook struct {
	hookType      reflect.Type
	hookPos       HookPos
	expectedCalls []hookCall
}

// NewMockHook returns a new MockHook object
func NewMockHook(hookType reflect.Type, pos HookPos) *MockHook {
	return &MockHook{hookType, pos, make([]hookCall, 0)}
}

func (h MockHook) Type() reflect.Type {
	return h.hookType
}

func (h MockHook) Pos() HookPos {
	return h.hookPos
}

func (h *MockHook) ExpectHookCall(item interface{}, domain Hookable) {
	h.expectedCalls = append(h.expectedCalls, hookCall{item, domain})
}

func (h *MockHook) AllExpectedCalled() {
	gomega.Expect(h.expectedCalls).To(gomega.BeEmpty())
}

func (h *MockHook) Func(item interface{}, domain Hookable, info interface{}) {
	gomega.Expect(h.expectedCalls).NotTo(gomega.BeEmpty())
	gomega.Expect(h.expectedCalls[0].item).To(gomega.BeIdenticalTo(item))
	gomega.Expect(h.expectedCalls[0].domain).To(gomega.BeIdenticalTo(domain))
	h.expectedCalls = h.expectedCalls[1:]
}

type someType struct {
}

type someType2 int32

var _ = Describe("BasicHookable", func() {
	It("should allow basic hooking", func() {
		domain := NewHookableBase()
		hook := NewMockHook(reflect.TypeOf((*someType)(nil)), BeforeEvent)
		domain.AcceptHook(hook)

		item := new(someType)

		hook.ExpectHookCall(item, domain)

		domain.InvokeHook(item, domain, BeforeEvent, nil)

		hook.AllExpectedCalled()
	})

	It("should not invoke if not hooking at the exact position", func() {
		domain := NewHookableBase()
		hook := NewMockHook(reflect.TypeOf((*someType)(nil)), BeforeEvent)
		domain.AcceptHook(hook)

		item := new(someType)

		domain.InvokeHook(item, domain, AfterEvent, nil)
	})

	It("should allow any position hooking", func() {
		domain := NewHookableBase()
		hook := NewMockHook(reflect.TypeOf((*someType)(nil)), Any)
		domain.AcceptHook(hook)

		item := new(someType)

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item, domain)
		domain.InvokeHook(item, domain, AfterEvent, nil)
		domain.InvokeHook(item, domain, BeforeEvent, nil)
		hook.AllExpectedCalled()
	})

	It("should allow hooking on an interface", func() {
		domain := NewHookableBase()
		hook := NewMockHook(reflect.TypeOf((*interface{})(nil)), AfterEvent)
		domain.AcceptHook(hook)

		item := new(someType)
		item2 := new(someType2)

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item2, domain)
		domain.InvokeHook(item, domain, AfterEvent, nil)
		domain.InvokeHook(item2, domain, AfterEvent, nil)
		hook.AllExpectedCalled()
	})

	It("should allow hooking on any type", func() {
		domain := NewHookableBase()
		hook := NewMockHook(nil, AfterEvent)
		domain.AcceptHook(hook)

		item := new(someType)
		item2 := new(someType2)
		item3 := 12

		hook.ExpectHookCall(item, domain)
		hook.ExpectHookCall(item2, domain)
		hook.ExpectHookCall(item3, domain)
		domain.InvokeHook(item, domain, AfterEvent, nil)
		domain.InvokeHook(item2, domain, AfterEvent, nil)
		domain.InvokeHook(item3, domain, AfterEvent, nil)
		hook.AllExpectedCalled()
	})

})
