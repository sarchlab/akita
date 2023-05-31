// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/sarchlab/akita/v3/mem/dram/internal/trans (interfaces: SubTransactionQueue,SubTransSplitter)

package dram

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	sim "github.com/sarchlab/akita/v3/sim"
	signal "github.com/sarchlab/akita/v3/mem/dram/internal/signal"
)

// MockSubTransactionQueue is a mock of SubTransactionQueue interface.
type MockSubTransactionQueue struct {
	ctrl     *gomock.Controller
	recorder *MockSubTransactionQueueMockRecorder
}

// MockSubTransactionQueueMockRecorder is the mock recorder for MockSubTransactionQueue.
type MockSubTransactionQueueMockRecorder struct {
	mock *MockSubTransactionQueue
}

// NewMockSubTransactionQueue creates a new mock instance.
func NewMockSubTransactionQueue(ctrl *gomock.Controller) *MockSubTransactionQueue {
	mock := &MockSubTransactionQueue{ctrl: ctrl}
	mock.recorder = &MockSubTransactionQueueMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSubTransactionQueue) EXPECT() *MockSubTransactionQueueMockRecorder {
	return m.recorder
}

// CanPush mocks base method.
func (m *MockSubTransactionQueue) CanPush(arg0 int) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CanPush", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// CanPush indicates an expected call of CanPush.
func (mr *MockSubTransactionQueueMockRecorder) CanPush(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CanPush", reflect.TypeOf((*MockSubTransactionQueue)(nil).CanPush), arg0)
}

// Push mocks base method.
func (m *MockSubTransactionQueue) Push(arg0 *signal.Transaction) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Push", arg0)
}

// Push indicates an expected call of Push.
func (mr *MockSubTransactionQueueMockRecorder) Push(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Push", reflect.TypeOf((*MockSubTransactionQueue)(nil).Push), arg0)
}

// Tick mocks base method.
func (m *MockSubTransactionQueue) Tick(arg0 sim.VTimeInSec) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Tick", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Tick indicates an expected call of Tick.
func (mr *MockSubTransactionQueueMockRecorder) Tick(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Tick", reflect.TypeOf((*MockSubTransactionQueue)(nil).Tick), arg0)
}

// MockSubTransSplitter is a mock of SubTransSplitter interface.
type MockSubTransSplitter struct {
	ctrl     *gomock.Controller
	recorder *MockSubTransSplitterMockRecorder
}

// MockSubTransSplitterMockRecorder is the mock recorder for MockSubTransSplitter.
type MockSubTransSplitterMockRecorder struct {
	mock *MockSubTransSplitter
}

// NewMockSubTransSplitter creates a new mock instance.
func NewMockSubTransSplitter(ctrl *gomock.Controller) *MockSubTransSplitter {
	mock := &MockSubTransSplitter{ctrl: ctrl}
	mock.recorder = &MockSubTransSplitterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSubTransSplitter) EXPECT() *MockSubTransSplitterMockRecorder {
	return m.recorder
}

// Split mocks base method.
func (m *MockSubTransSplitter) Split(arg0 *signal.Transaction) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Split", arg0)
}

// Split indicates an expected call of Split.
func (mr *MockSubTransSplitterMockRecorder) Split(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Split", reflect.TypeOf((*MockSubTransSplitter)(nil).Split), arg0)
}
