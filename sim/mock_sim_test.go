// Code generated by MockGen. DO NOT EDIT.
// Source: github/sarchlab/akita/v3/sim (interfaces: Port,Engine,Event,Connection,Component,Handler,Ticker,Buffer)

package sim

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockPort is a mock of Port interface.
type MockPort struct {
	ctrl     *gomock.Controller
	recorder *MockPortMockRecorder
}

// MockPortMockRecorder is the mock recorder for MockPort.
type MockPortMockRecorder struct {
	mock *MockPort
}

// NewMockPort creates a new mock instance.
func NewMockPort(ctrl *gomock.Controller) *MockPort {
	mock := &MockPort{ctrl: ctrl}
	mock.recorder = &MockPortMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPort) EXPECT() *MockPortMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockPort) AcceptHook(arg0 Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", arg0)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockPortMockRecorder) AcceptHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockPort)(nil).AcceptHook), arg0)
}

// CanSend mocks base method.
func (m *MockPort) CanSend() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CanSend")
	ret0, _ := ret[0].(bool)
	return ret0
}

// CanSend indicates an expected call of CanSend.
func (mr *MockPortMockRecorder) CanSend() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CanSend", reflect.TypeOf((*MockPort)(nil).CanSend))
}

// Component mocks base method.
func (m *MockPort) Component() Component {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Component")
	ret0, _ := ret[0].(Component)
	return ret0
}

// Component indicates an expected call of Component.
func (mr *MockPortMockRecorder) Component() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Component", reflect.TypeOf((*MockPort)(nil).Component))
}

// Hooks mocks base method.
func (m *MockPort) Hooks() []Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockPortMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockPort)(nil).Hooks))
}

// Name mocks base method.
func (m *MockPort) Name() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name.
func (mr *MockPortMockRecorder) Name() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Name", reflect.TypeOf((*MockPort)(nil).Name))
}

// NotifyAvailable mocks base method.
func (m *MockPort) NotifyAvailable(arg0 VTimeInSec) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "NotifyAvailable", arg0)
}

// NotifyAvailable indicates an expected call of NotifyAvailable.
func (mr *MockPortMockRecorder) NotifyAvailable(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NotifyAvailable", reflect.TypeOf((*MockPort)(nil).NotifyAvailable), arg0)
}

// NumHooks mocks base method.
func (m *MockPort) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockPortMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockPort)(nil).NumHooks))
}

// Peek mocks base method.
func (m *MockPort) Peek() Msg {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Peek")
	ret0, _ := ret[0].(Msg)
	return ret0
}

// Peek indicates an expected call of Peek.
func (mr *MockPortMockRecorder) Peek() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Peek", reflect.TypeOf((*MockPort)(nil).Peek))
}

// Recv mocks base method.
func (m *MockPort) Recv(arg0 Msg) *SendError {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Recv", arg0)
	ret0, _ := ret[0].(*SendError)
	return ret0
}

// Recv indicates an expected call of Recv.
func (mr *MockPortMockRecorder) Recv(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Recv", reflect.TypeOf((*MockPort)(nil).Recv), arg0)
}

// Retrieve mocks base method.
func (m *MockPort) Retrieve(arg0 VTimeInSec) Msg {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Retrieve", arg0)
	ret0, _ := ret[0].(Msg)
	return ret0
}

// Retrieve indicates an expected call of Retrieve.
func (mr *MockPortMockRecorder) Retrieve(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Retrieve", reflect.TypeOf((*MockPort)(nil).Retrieve), arg0)
}

// Send mocks base method.
func (m *MockPort) Send(arg0 Msg) *SendError {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Send", arg0)
	ret0, _ := ret[0].(*SendError)
	return ret0
}

// Send indicates an expected call of Send.
func (mr *MockPortMockRecorder) Send(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Send", reflect.TypeOf((*MockPort)(nil).Send), arg0)
}

// SetConnection mocks base method.
func (m *MockPort) SetConnection(arg0 Connection) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetConnection", arg0)
}

// SetConnection indicates an expected call of SetConnection.
func (mr *MockPortMockRecorder) SetConnection(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetConnection", reflect.TypeOf((*MockPort)(nil).SetConnection), arg0)
}

// MockEngine is a mock of Engine interface.
type MockEngine struct {
	ctrl     *gomock.Controller
	recorder *MockEngineMockRecorder
}

// MockEngineMockRecorder is the mock recorder for MockEngine.
type MockEngineMockRecorder struct {
	mock *MockEngine
}

// NewMockEngine creates a new mock instance.
func NewMockEngine(ctrl *gomock.Controller) *MockEngine {
	mock := &MockEngine{ctrl: ctrl}
	mock.recorder = &MockEngineMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockEngine) EXPECT() *MockEngineMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockEngine) AcceptHook(arg0 Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", arg0)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockEngineMockRecorder) AcceptHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockEngine)(nil).AcceptHook), arg0)
}

// Continue mocks base method.
func (m *MockEngine) Continue() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Continue")
}

// Continue indicates an expected call of Continue.
func (mr *MockEngineMockRecorder) Continue() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Continue", reflect.TypeOf((*MockEngine)(nil).Continue))
}

// CurrentTime mocks base method.
func (m *MockEngine) CurrentTime() VTimeInSec {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CurrentTime")
	ret0, _ := ret[0].(VTimeInSec)
	return ret0
}

// CurrentTime indicates an expected call of CurrentTime.
func (mr *MockEngineMockRecorder) CurrentTime() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CurrentTime", reflect.TypeOf((*MockEngine)(nil).CurrentTime))
}

// Finished mocks base method.
func (m *MockEngine) Finished() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Finished")
}

// Finished indicates an expected call of Finished.
func (mr *MockEngineMockRecorder) Finished() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Finished", reflect.TypeOf((*MockEngine)(nil).Finished))
}

// Hooks mocks base method.
func (m *MockEngine) Hooks() []Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockEngineMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockEngine)(nil).Hooks))
}

// NumHooks mocks base method.
func (m *MockEngine) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockEngineMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockEngine)(nil).NumHooks))
}

// Pause mocks base method.
func (m *MockEngine) Pause() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Pause")
}

// Pause indicates an expected call of Pause.
func (mr *MockEngineMockRecorder) Pause() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Pause", reflect.TypeOf((*MockEngine)(nil).Pause))
}

// RegisterSimulationEndHandler mocks base method.
func (m *MockEngine) RegisterSimulationEndHandler(arg0 SimulationEndHandler) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RegisterSimulationEndHandler", arg0)
}

// RegisterSimulationEndHandler indicates an expected call of RegisterSimulationEndHandler.
func (mr *MockEngineMockRecorder) RegisterSimulationEndHandler(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterSimulationEndHandler", reflect.TypeOf((*MockEngine)(nil).RegisterSimulationEndHandler), arg0)
}

// Run mocks base method.
func (m *MockEngine) Run() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Run")
	ret0, _ := ret[0].(error)
	return ret0
}

// Run indicates an expected call of Run.
func (mr *MockEngineMockRecorder) Run() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Run", reflect.TypeOf((*MockEngine)(nil).Run))
}

// Schedule mocks base method.
func (m *MockEngine) Schedule(arg0 Event) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Schedule", arg0)
}

// Schedule indicates an expected call of Schedule.
func (mr *MockEngineMockRecorder) Schedule(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Schedule", reflect.TypeOf((*MockEngine)(nil).Schedule), arg0)
}

// MockEvent is a mock of Event interface.
type MockEvent struct {
	ctrl     *gomock.Controller
	recorder *MockEventMockRecorder
}

// MockEventMockRecorder is the mock recorder for MockEvent.
type MockEventMockRecorder struct {
	mock *MockEvent
}

// NewMockEvent creates a new mock instance.
func NewMockEvent(ctrl *gomock.Controller) *MockEvent {
	mock := &MockEvent{ctrl: ctrl}
	mock.recorder = &MockEventMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockEvent) EXPECT() *MockEventMockRecorder {
	return m.recorder
}

// Handler mocks base method.
func (m *MockEvent) Handler() Handler {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Handler")
	ret0, _ := ret[0].(Handler)
	return ret0
}

// Handler indicates an expected call of Handler.
func (mr *MockEventMockRecorder) Handler() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Handler", reflect.TypeOf((*MockEvent)(nil).Handler))
}

// IsSecondary mocks base method.
func (m *MockEvent) IsSecondary() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsSecondary")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsSecondary indicates an expected call of IsSecondary.
func (mr *MockEventMockRecorder) IsSecondary() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsSecondary", reflect.TypeOf((*MockEvent)(nil).IsSecondary))
}

// Time mocks base method.
func (m *MockEvent) Time() VTimeInSec {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Time")
	ret0, _ := ret[0].(VTimeInSec)
	return ret0
}

// Time indicates an expected call of Time.
func (mr *MockEventMockRecorder) Time() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Time", reflect.TypeOf((*MockEvent)(nil).Time))
}

// MockConnection is a mock of Connection interface.
type MockConnection struct {
	ctrl     *gomock.Controller
	recorder *MockConnectionMockRecorder
}

// MockConnectionMockRecorder is the mock recorder for MockConnection.
type MockConnectionMockRecorder struct {
	mock *MockConnection
}

// NewMockConnection creates a new mock instance.
func NewMockConnection(ctrl *gomock.Controller) *MockConnection {
	mock := &MockConnection{ctrl: ctrl}
	mock.recorder = &MockConnectionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConnection) EXPECT() *MockConnectionMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockConnection) AcceptHook(arg0 Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", arg0)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockConnectionMockRecorder) AcceptHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockConnection)(nil).AcceptHook), arg0)
}

// CanSend mocks base method.
func (m *MockConnection) CanSend(arg0 Port) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CanSend", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// CanSend indicates an expected call of CanSend.
func (mr *MockConnectionMockRecorder) CanSend(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CanSend", reflect.TypeOf((*MockConnection)(nil).CanSend), arg0)
}

// Hooks mocks base method.
func (m *MockConnection) Hooks() []Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockConnectionMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockConnection)(nil).Hooks))
}

// NotifyAvailable mocks base method.
func (m *MockConnection) NotifyAvailable(arg0 VTimeInSec, arg1 Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "NotifyAvailable", arg0, arg1)
}

// NotifyAvailable indicates an expected call of NotifyAvailable.
func (mr *MockConnectionMockRecorder) NotifyAvailable(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NotifyAvailable", reflect.TypeOf((*MockConnection)(nil).NotifyAvailable), arg0, arg1)
}

// NumHooks mocks base method.
func (m *MockConnection) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockConnectionMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockConnection)(nil).NumHooks))
}

// PlugIn mocks base method.
func (m *MockConnection) PlugIn(arg0 Port, arg1 int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "PlugIn", arg0, arg1)
}

// PlugIn indicates an expected call of PlugIn.
func (mr *MockConnectionMockRecorder) PlugIn(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PlugIn", reflect.TypeOf((*MockConnection)(nil).PlugIn), arg0, arg1)
}

// Send mocks base method.
func (m *MockConnection) Send(arg0 Msg) *SendError {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Send", arg0)
	ret0, _ := ret[0].(*SendError)
	return ret0
}

// Send indicates an expected call of Send.
func (mr *MockConnectionMockRecorder) Send(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Send", reflect.TypeOf((*MockConnection)(nil).Send), arg0)
}

// Unplug mocks base method.
func (m *MockConnection) Unplug(arg0 Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Unplug", arg0)
}

// Unplug indicates an expected call of Unplug.
func (mr *MockConnectionMockRecorder) Unplug(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Unplug", reflect.TypeOf((*MockConnection)(nil).Unplug), arg0)
}

// MockComponent is a mock of Component interface.
type MockComponent struct {
	ctrl     *gomock.Controller
	recorder *MockComponentMockRecorder
}

// MockComponentMockRecorder is the mock recorder for MockComponent.
type MockComponentMockRecorder struct {
	mock *MockComponent
}

// NewMockComponent creates a new mock instance.
func NewMockComponent(ctrl *gomock.Controller) *MockComponent {
	mock := &MockComponent{ctrl: ctrl}
	mock.recorder = &MockComponentMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockComponent) EXPECT() *MockComponentMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockComponent) AcceptHook(arg0 Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", arg0)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockComponentMockRecorder) AcceptHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockComponent)(nil).AcceptHook), arg0)
}

// AddPort mocks base method.
func (m *MockComponent) AddPort(arg0 string, arg1 Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddPort", arg0, arg1)
}

// AddPort indicates an expected call of AddPort.
func (mr *MockComponentMockRecorder) AddPort(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddPort", reflect.TypeOf((*MockComponent)(nil).AddPort), arg0, arg1)
}

// GetPortByName mocks base method.
func (m *MockComponent) GetPortByName(arg0 string) Port {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPortByName", arg0)
	ret0, _ := ret[0].(Port)
	return ret0
}

// GetPortByName indicates an expected call of GetPortByName.
func (mr *MockComponentMockRecorder) GetPortByName(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPortByName", reflect.TypeOf((*MockComponent)(nil).GetPortByName), arg0)
}

// Handle mocks base method.
func (m *MockComponent) Handle(arg0 Event) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Handle", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Handle indicates an expected call of Handle.
func (mr *MockComponentMockRecorder) Handle(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Handle", reflect.TypeOf((*MockComponent)(nil).Handle), arg0)
}

// Hooks mocks base method.
func (m *MockComponent) Hooks() []Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockComponentMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockComponent)(nil).Hooks))
}

// Name mocks base method.
func (m *MockComponent) Name() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name.
func (mr *MockComponentMockRecorder) Name() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Name", reflect.TypeOf((*MockComponent)(nil).Name))
}

// NotifyPortFree mocks base method.
func (m *MockComponent) NotifyPortFree(arg0 VTimeInSec, arg1 Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "NotifyPortFree", arg0, arg1)
}

// NotifyPortFree indicates an expected call of NotifyPortFree.
func (mr *MockComponentMockRecorder) NotifyPortFree(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NotifyPortFree", reflect.TypeOf((*MockComponent)(nil).NotifyPortFree), arg0, arg1)
}

// NotifyRecv mocks base method.
func (m *MockComponent) NotifyRecv(arg0 VTimeInSec, arg1 Port) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "NotifyRecv", arg0, arg1)
}

// NotifyRecv indicates an expected call of NotifyRecv.
func (mr *MockComponentMockRecorder) NotifyRecv(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NotifyRecv", reflect.TypeOf((*MockComponent)(nil).NotifyRecv), arg0, arg1)
}

// NumHooks mocks base method.
func (m *MockComponent) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockComponentMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockComponent)(nil).NumHooks))
}

// Ports mocks base method.
func (m *MockComponent) Ports() []Port {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ports")
	ret0, _ := ret[0].([]Port)
	return ret0
}

// Ports indicates an expected call of Ports.
func (mr *MockComponentMockRecorder) Ports() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ports", reflect.TypeOf((*MockComponent)(nil).Ports))
}

// MockHandler is a mock of Handler interface.
type MockHandler struct {
	ctrl     *gomock.Controller
	recorder *MockHandlerMockRecorder
}

// MockHandlerMockRecorder is the mock recorder for MockHandler.
type MockHandlerMockRecorder struct {
	mock *MockHandler
}

// NewMockHandler creates a new mock instance.
func NewMockHandler(ctrl *gomock.Controller) *MockHandler {
	mock := &MockHandler{ctrl: ctrl}
	mock.recorder = &MockHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHandler) EXPECT() *MockHandlerMockRecorder {
	return m.recorder
}

// Handle mocks base method.
func (m *MockHandler) Handle(arg0 Event) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Handle", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Handle indicates an expected call of Handle.
func (mr *MockHandlerMockRecorder) Handle(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Handle", reflect.TypeOf((*MockHandler)(nil).Handle), arg0)
}

// MockTicker is a mock of Ticker interface.
type MockTicker struct {
	ctrl     *gomock.Controller
	recorder *MockTickerMockRecorder
}

// MockTickerMockRecorder is the mock recorder for MockTicker.
type MockTickerMockRecorder struct {
	mock *MockTicker
}

// NewMockTicker creates a new mock instance.
func NewMockTicker(ctrl *gomock.Controller) *MockTicker {
	mock := &MockTicker{ctrl: ctrl}
	mock.recorder = &MockTickerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTicker) EXPECT() *MockTickerMockRecorder {
	return m.recorder
}

// Tick mocks base method.
func (m *MockTicker) Tick(arg0 VTimeInSec) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Tick", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Tick indicates an expected call of Tick.
func (mr *MockTickerMockRecorder) Tick(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Tick", reflect.TypeOf((*MockTicker)(nil).Tick), arg0)
}

// MockBuffer is a mock of Buffer interface.
type MockBuffer struct {
	ctrl     *gomock.Controller
	recorder *MockBufferMockRecorder
}

// MockBufferMockRecorder is the mock recorder for MockBuffer.
type MockBufferMockRecorder struct {
	mock *MockBuffer
}

// NewMockBuffer creates a new mock instance.
func NewMockBuffer(ctrl *gomock.Controller) *MockBuffer {
	mock := &MockBuffer{ctrl: ctrl}
	mock.recorder = &MockBufferMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBuffer) EXPECT() *MockBufferMockRecorder {
	return m.recorder
}

// AcceptHook mocks base method.
func (m *MockBuffer) AcceptHook(arg0 Hook) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AcceptHook", arg0)
}

// AcceptHook indicates an expected call of AcceptHook.
func (mr *MockBufferMockRecorder) AcceptHook(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AcceptHook", reflect.TypeOf((*MockBuffer)(nil).AcceptHook), arg0)
}

// CanPush mocks base method.
func (m *MockBuffer) CanPush() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CanPush")
	ret0, _ := ret[0].(bool)
	return ret0
}

// CanPush indicates an expected call of CanPush.
func (mr *MockBufferMockRecorder) CanPush() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CanPush", reflect.TypeOf((*MockBuffer)(nil).CanPush))
}

// Capacity mocks base method.
func (m *MockBuffer) Capacity() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Capacity")
	ret0, _ := ret[0].(int)
	return ret0
}

// Capacity indicates an expected call of Capacity.
func (mr *MockBufferMockRecorder) Capacity() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Capacity", reflect.TypeOf((*MockBuffer)(nil).Capacity))
}

// Clear mocks base method.
func (m *MockBuffer) Clear() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Clear")
}

// Clear indicates an expected call of Clear.
func (mr *MockBufferMockRecorder) Clear() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Clear", reflect.TypeOf((*MockBuffer)(nil).Clear))
}

// Hooks mocks base method.
func (m *MockBuffer) Hooks() []Hook {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Hooks")
	ret0, _ := ret[0].([]Hook)
	return ret0
}

// Hooks indicates an expected call of Hooks.
func (mr *MockBufferMockRecorder) Hooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Hooks", reflect.TypeOf((*MockBuffer)(nil).Hooks))
}

// Name mocks base method.
func (m *MockBuffer) Name() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name.
func (mr *MockBufferMockRecorder) Name() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Name", reflect.TypeOf((*MockBuffer)(nil).Name))
}

// NumHooks mocks base method.
func (m *MockBuffer) NumHooks() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumHooks")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumHooks indicates an expected call of NumHooks.
func (mr *MockBufferMockRecorder) NumHooks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumHooks", reflect.TypeOf((*MockBuffer)(nil).NumHooks))
}

// Peek mocks base method.
func (m *MockBuffer) Peek() interface{} {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Peek")
	ret0, _ := ret[0].(interface{})
	return ret0
}

// Peek indicates an expected call of Peek.
func (mr *MockBufferMockRecorder) Peek() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Peek", reflect.TypeOf((*MockBuffer)(nil).Peek))
}

// Pop mocks base method.
func (m *MockBuffer) Pop() interface{} {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Pop")
	ret0, _ := ret[0].(interface{})
	return ret0
}

// Pop indicates an expected call of Pop.
func (mr *MockBufferMockRecorder) Pop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Pop", reflect.TypeOf((*MockBuffer)(nil).Pop))
}

// Push mocks base method.
func (m *MockBuffer) Push(arg0 interface{}) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Push", arg0)
}

// Push indicates an expected call of Push.
func (mr *MockBufferMockRecorder) Push(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Push", reflect.TypeOf((*MockBuffer)(nil).Push), arg0)
}

// Size mocks base method.
func (m *MockBuffer) Size() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Size")
	ret0, _ := ret[0].(int)
	return ret0
}

// Size indicates an expected call of Size.
func (mr *MockBufferMockRecorder) Size() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Size", reflect.TypeOf((*MockBuffer)(nil).Size))
}
