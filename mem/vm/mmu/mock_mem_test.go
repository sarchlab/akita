// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/sarchlab/akita/v3/mem/mem (interfaces: LowModuleFinder)

package mmu

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	sim "github.com/sarchlab/akita/v3/sim"
)

// MockLowModuleFinder is a mock of LowModuleFinder interface.
type MockLowModuleFinder struct {
	ctrl     *gomock.Controller
	recorder *MockLowModuleFinderMockRecorder
}

// MockLowModuleFinderMockRecorder is the mock recorder for MockLowModuleFinder.
type MockLowModuleFinderMockRecorder struct {
	mock *MockLowModuleFinder
}

// NewMockLowModuleFinder creates a new mock instance.
func NewMockLowModuleFinder(ctrl *gomock.Controller) *MockLowModuleFinder {
	mock := &MockLowModuleFinder{ctrl: ctrl}
	mock.recorder = &MockLowModuleFinderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLowModuleFinder) EXPECT() *MockLowModuleFinderMockRecorder {
	return m.recorder
}

// Find mocks base method.
func (m *MockLowModuleFinder) Find(arg0 uint64) sim.Port {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Find", arg0)
	ret0, _ := ret[0].(sim.Port)
	return ret0
}

// Find indicates an expected call of Find.
func (mr *MockLowModuleFinderMockRecorder) Find(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Find", reflect.TypeOf((*MockLowModuleFinder)(nil).Find), arg0)
}
