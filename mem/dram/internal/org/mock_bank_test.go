// Code generated by MockGen. DO NOT EDIT.
// Source: bank.go

// Package org is a generated GoMock package.
package org

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	signal "github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	sim "github.com/sarchlab/akita/v4/sim"
)

// MockBank is a mock of Bank interface.
type MockBank struct {
	ctrl     *gomock.Controller
	recorder *MockBankMockRecorder
}

// MockBankMockRecorder is the mock recorder for MockBank.
type MockBankMockRecorder struct {
	mock *MockBank
}

// NewMockBank creates a new mock instance.
func NewMockBank(ctrl *gomock.Controller) *MockBank {
	mock := &MockBank{ctrl: ctrl}
	mock.recorder = &MockBankMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBank) EXPECT() *MockBankMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockBank) AcceptHook(hook sim.Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", hook)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockBankMockRecorder) AcceptHook(hook interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockBank)(nil).AcceptHook), hook)
}

// GetReadyCommand mocks base method.
func (m *MockBank) GetReadyCommand(now sim.VTimeInSec, cmd *signal.Command) *signal.Command {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetReadyCommand", now, cmd)
	ret0, _ := ret[0].(*signal.Command)
	return ret0
}

// GetReadyCommand indicates an expected call of GetReadyCommand.
func (mr *MockBankMockRecorder) GetReadyCommand(now, cmd interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetReadyCommand", reflect.TypeOf((*MockBank)(nil).GetReadyCommand), now, cmd)
}

// Hooks mocks base method.
func (m *MockBank) Hooks() []sim.Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]sim.Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockBankMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockBank)(nil).Hooks))
}

// InvokeHook mocks base method.
func (m *MockBank) InvokeHook(arg0 sim.HookCtx) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "InvokeHook", arg0)
}

// InvokeHook indicates an expected call of InvokeHook.
func (mr *MockBankMockRecorder) InvokeHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InvokeHook", reflect.TypeOf((*MockBank)(nil).InvokeHook), arg0)
}

// Name mocks base method.
func (m *MockBank) Name() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name.
func (mr *MockBankMockRecorder) Name() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Name", reflect.TypeOf((*MockBank)(nil).Name))
}

// NumHooks mocks base method.
func (m *MockBank) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockBankMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockBank)(nil).NumHooks))
}

// StartCommand mocks base method.
func (m *MockBank) StartCommand(now sim.VTimeInSec, cmd *signal.Command) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "StartCommand", now, cmd)
}

// StartCommand indicates an expected call of StartCommand.
func (mr *MockBankMockRecorder) StartCommand(now, cmd interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StartCommand", reflect.TypeOf((*MockBank)(nil).StartCommand), now, cmd)
}

// Tick mocks base method.
func (m *MockBank) Tick(now sim.VTimeInSec) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Tick", now)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Tick indicates an expected call of Tick.
func (mr *MockBankMockRecorder) Tick(now interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Tick", reflect.TypeOf((*MockBank)(nil).Tick), now)
}

// UpdateTiming mocks base method.
func (m *MockBank) UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "UpdateTiming", cmdKind, cycleNeeded)
}

// UpdateTiming indicates an expected call of UpdateTiming.
func (mr *MockBankMockRecorder) UpdateTiming(cmdKind, cycleNeeded interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateTiming", reflect.TypeOf((*MockBank)(nil).UpdateTiming), cmdKind, cycleNeeded)
}
