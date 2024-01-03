// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/sarchlab/akita/v4/noc/networking/routing (interfaces: Table)

package switching

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	sim "github.com/sarchlab/akita/v4/sim"
)

// MockTable is a mock of Table interface.
type MockTable struct {
	ctrl     *gomock.Controller
	recorder *MockTableMockRecorder
}

// MockTableMockRecorder is the mock recorder for MockTable.
type MockTableMockRecorder struct {
	mock *MockTable
}

// NewMockTable creates a new mock instance.
func NewMockTable(ctrl *gomock.Controller) *MockTable {
	mock := &MockTable{ctrl: ctrl}
	mock.recorder = &MockTableMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTable) EXPECT() *MockTableMockRecorder {
	return m.recorder
}

// DefineDefaultRoute mocks base method.
func (m *MockTable) DefineDefaultRoute(arg0 sim.Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DefineDefaultRoute", arg0)
}

// DefineDefaultRoute indicates an expected call of DefineDefaultRoute.
func (mr *MockTableMockRecorder) DefineDefaultRoute(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DefineDefaultRoute", reflect.TypeOf((*MockTable)(nil).DefineDefaultRoute), arg0)
}

// DefineRoute mocks base method.
func (m *MockTable) DefineRoute(arg0, arg1 sim.Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DefineRoute", arg0, arg1)
}

// DefineRoute indicates an expected call of DefineRoute.
func (mr *MockTableMockRecorder) DefineRoute(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DefineRoute", reflect.TypeOf((*MockTable)(nil).DefineRoute), arg0, arg1)
}

// FindPort mocks base method.
func (m *MockTable) FindPort(arg0 sim.Port) sim.Port {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindPort", arg0)
	ret0, _ := ret[0].(sim.Port)
	return ret0
}

// FindPort indicates an expected call of FindPort.
func (mr *MockTableMockRecorder) FindPort(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindPort", reflect.TypeOf((*MockTable)(nil).FindPort), arg0)
}
