package mem

import (
	"reflect"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

var accessReqByteOverhead = 12
var accessRspByteOverhead = 4
var controlMsgByteOverhead = 4

// AccessReqPayload abstracts read and write request payloads that are sent to
// the cache modules or memory controllers.
type AccessReqPayload interface {
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// AccessRspPayload abstracts response payloads in the memory system.
type AccessRspPayload interface{}

// ReadReqPayload is the payload for a read request sent to a memory controller.
type ReadReqPayload struct {
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// GetByteSize returns the number of byte that the request is accessing.
func (r *ReadReqPayload) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing.
func (r *ReadReqPayload) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r *ReadReqPayload) GetPID() vm.PID {
	return r.PID
}

// ReadReqBuilder can build read requests.
type ReadReqBuilder struct {
	src, dst           sim.RemotePort
	pid                vm.PID
	address, byteSize  uint64
	canWaitForCoalesce bool
	info               interface{}
}

// WithSrc sets the source of the request to build.
func (b ReadReqBuilder) WithSrc(src sim.RemotePort) ReadReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b ReadReqBuilder) WithDst(dst sim.RemotePort) ReadReqBuilder {
	b.dst = dst
	return b
}

// WithPID sets the PID of the request to build.
func (b ReadReqBuilder) WithPID(pid vm.PID) ReadReqBuilder {
	b.pid = pid
	return b
}

// WithInfo sets the Info of the request to build.
func (b ReadReqBuilder) WithInfo(info interface{}) ReadReqBuilder {
	b.info = info
	return b
}

// WithAddress sets the address of the request to build.
func (b ReadReqBuilder) WithAddress(address uint64) ReadReqBuilder {
	b.address = address
	return b
}

// WithByteSize sets the byte size of the request to build.
func (b ReadReqBuilder) WithByteSize(byteSize uint64) ReadReqBuilder {
	b.byteSize = byteSize
	return b
}

// CanWaitForCoalesce allow the request to build to wait for coalesce.
func (b ReadReqBuilder) CanWaitForCoalesce() ReadReqBuilder {
	b.canWaitForCoalesce = true
	return b
}

// Build creates a new *sim.GenericMsg with ReadReqPayload.
func (b ReadReqBuilder) Build() *sim.GenericMsg {
	payload := &ReadReqPayload{
		Address:            b.address,
		AccessByteSize:     b.byteSize,
		PID:                b.pid,
		CanWaitForCoalesce: b.canWaitForCoalesce,
		Info:               b.info,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficBytes: accessReqByteOverhead,
			TrafficClass: reflect.TypeOf(ReadReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// WriteReqPayload is the payload for a write request sent to a memory
// controller.
type WriteReqPayload struct {
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// GetByteSize returns the number of byte that the request is writing.
func (r *WriteReqPayload) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing.
func (r *WriteReqPayload) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address.
func (r *WriteReqPayload) GetPID() vm.PID {
	return r.PID
}

// WriteReqBuilder can build write requests.
type WriteReqBuilder struct {
	src, dst           sim.RemotePort
	pid                vm.PID
	info               interface{}
	address            uint64
	data               []byte
	dirtyMask          []bool
	canWaitForCoalesce bool
}

// WithSrc sets the source of the request to build.
func (b WriteReqBuilder) WithSrc(src sim.RemotePort) WriteReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b WriteReqBuilder) WithDst(dst sim.RemotePort) WriteReqBuilder {
	b.dst = dst
	return b
}

// WithPID sets the PID of the request to build.
func (b WriteReqBuilder) WithPID(pid vm.PID) WriteReqBuilder {
	b.pid = pid
	return b
}

// WithInfo sets the information attached to the request to build.
func (b WriteReqBuilder) WithInfo(info interface{}) WriteReqBuilder {
	b.info = info
	return b
}

// WithAddress sets the address of the request to build.
func (b WriteReqBuilder) WithAddress(address uint64) WriteReqBuilder {
	b.address = address
	return b
}

// WithData sets the data of the request to build.
func (b WriteReqBuilder) WithData(data []byte) WriteReqBuilder {
	b.data = data
	return b
}

// WithDirtyMask sets the dirty mask of the request to build.
func (b WriteReqBuilder) WithDirtyMask(mask []bool) WriteReqBuilder {
	b.dirtyMask = mask
	return b
}

// CanWaitForCoalesce allow the request to build to wait for coalesce.
func (b WriteReqBuilder) CanWaitForCoalesce() WriteReqBuilder {
	b.canWaitForCoalesce = true
	return b
}

// Build creates a new *sim.GenericMsg with WriteReqPayload.
func (b WriteReqBuilder) Build() *sim.GenericMsg {
	payload := &WriteReqPayload{
		Address:            b.address,
		Data:               b.data,
		DirtyMask:          b.dirtyMask,
		PID:                b.pid,
		CanWaitForCoalesce: b.canWaitForCoalesce,
		Info:               b.info,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficBytes: len(b.data) + accessReqByteOverhead,
			TrafficClass: reflect.TypeOf(WriteReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// DataReadyRspPayload is the payload for a response carrying data loaded from
// memory.
type DataReadyRspPayload struct {
	Data []byte
}

// DataReadyRspBuilder can build data ready responds.
type DataReadyRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
	data     []byte
}

// WithSrc sets the source of the request to build.
func (b DataReadyRspBuilder) WithSrc(src sim.RemotePort) DataReadyRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataReadyRspBuilder) WithDst(dst sim.RemotePort) DataReadyRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b DataReadyRspBuilder) WithRspTo(id string) DataReadyRspBuilder {
	b.rspTo = id
	return b
}

// WithData sets the data of the request to build.
func (b DataReadyRspBuilder) WithData(data []byte) DataReadyRspBuilder {
	b.data = data
	return b
}

// Build creates a new *sim.GenericMsg with DataReadyRspPayload.
func (b DataReadyRspBuilder) Build() *sim.GenericMsg {
	payload := &DataReadyRspPayload{
		Data: b.data,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			RspTo:        b.rspTo,
			TrafficBytes: len(b.data) + accessRspByteOverhead,
			TrafficClass: reflect.TypeOf(ReadReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// WriteDoneRspPayload is the payload for a response indicating a write
// request is completed.
type WriteDoneRspPayload struct{}

// WriteDoneRspBuilder can build write-done responds.
type WriteDoneRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b WriteDoneRspBuilder) WithSrc(src sim.RemotePort) WriteDoneRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b WriteDoneRspBuilder) WithDst(dst sim.RemotePort) WriteDoneRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b WriteDoneRspBuilder) WithRspTo(id string) WriteDoneRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new *sim.GenericMsg with WriteDoneRspPayload.
func (b WriteDoneRspBuilder) Build() *sim.GenericMsg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			RspTo:        b.rspTo,
			TrafficBytes: accessRspByteOverhead,
			TrafficClass: reflect.TypeOf(WriteReqPayload{}).String(),
		},
		Payload: &WriteDoneRspPayload{},
	}
}

// ControlMsgPayload is the payload for control messages used for managing
// components on the memory hierarchy.
type ControlMsgPayload struct {
	DiscardTransations bool
	Restart            bool
	NotifyDone         bool
	Enable             bool
	Drain              bool
	Flush              bool
	Pause              bool
	Invalid            bool
}

// ControlMsgBuilder can build control messages.
type ControlMsgBuilder struct {
	src, dst            sim.RemotePort
	discardTransactions bool
	restart             bool
	notifyDone          bool
	Enable              bool
	Drain               bool
	Flush               bool
	Pause               bool
	Invalid             bool
}

// WithSrc sets the source of the request to build.
func (b ControlMsgBuilder) WithSrc(src sim.RemotePort) ControlMsgBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b ControlMsgBuilder) WithDst(dst sim.RemotePort) ControlMsgBuilder {
	b.dst = dst
	return b
}

// ToDiscardTransactions sets the discard transactions bit of the control
// messages to 1.
func (b ControlMsgBuilder) ToDiscardTransactions() ControlMsgBuilder {
	b.discardTransactions = true
	return b
}

// ToRestart sets the restart bit of the control messages to 1.
func (b ControlMsgBuilder) ToRestart() ControlMsgBuilder {
	b.restart = true
	return b
}

// ToNotifyDone sets the "notify done" bit of the control messages to 1.
func (b ControlMsgBuilder) ToNotifyDone() ControlMsgBuilder {
	b.notifyDone = true
	return b
}

// WithCtrlInfo sets the enable bit of the control messages to 1.
func (b ControlMsgBuilder) WithCtrlInfo(
	enable bool, drain bool, flush bool, pause bool, invalid bool,
) ControlMsgBuilder {
	b.Enable = enable
	b.Drain = drain
	b.Flush = flush
	b.Pause = pause
	b.Invalid = invalid

	return b
}

// Build creates a new *sim.GenericMsg with ControlMsgPayload.
func (b ControlMsgBuilder) Build() *sim.GenericMsg {
	payload := &ControlMsgPayload{
		DiscardTransations: b.discardTransactions,
		Restart:            b.restart,
		NotifyDone:         b.notifyDone,
		Enable:             b.Enable,
		Drain:              b.Drain,
		Flush:              b.Flush,
		Pause:              b.Pause,
		Invalid:            b.Invalid,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficBytes: controlMsgByteOverhead,
			TrafficClass: reflect.TypeOf(ControlMsgPayload{}).String(),
		},
		Payload: payload,
	}
}

// ControlMsgRspPayload is the payload for control message responses.
type ControlMsgRspPayload struct {
	Enable  bool
	Drain   bool
	Flush   bool
	Pause   bool
	Invalid bool
}

// ControlMsgRspBuilder can build control message responses.
type ControlMsgRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
	enable   bool
	drain    bool
	flush    bool
	pause    bool
	invalid  bool
}

// WithSrc sets the source of the request to build.
func (b ControlMsgRspBuilder) WithSrc(src sim.RemotePort) ControlMsgRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b ControlMsgRspBuilder) WithDst(dst sim.RemotePort) ControlMsgRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b ControlMsgRspBuilder) WithRspTo(id string) ControlMsgRspBuilder {
	b.rspTo = id
	return b
}

// WithEnable sets the enable bit of the control messages to 1.
func (b ControlMsgRspBuilder) WithEnable(enable bool) ControlMsgRspBuilder {
	b.enable = enable
	return b
}

// WithDrain sets the drain bit of the control messages to 1.
func (b ControlMsgRspBuilder) WithDrain(drain bool) ControlMsgRspBuilder {
	b.drain = drain
	return b
}

// WithFlush sets the flush bit of the control messages to 1.
func (b ControlMsgRspBuilder) WithFlush(flush bool) ControlMsgRspBuilder {
	b.flush = flush
	return b
}

// WithPause sets the pause bit of the control messages to 1.
func (b ControlMsgRspBuilder) WithPause(pause bool) ControlMsgRspBuilder {
	b.pause = pause
	return b
}

// WithInvalid sets the invalid bit of the control messages to 1.
func (b ControlMsgRspBuilder) WithInvalid(invalid bool) ControlMsgRspBuilder {
	b.invalid = invalid
	return b
}

// Build creates a new *sim.GenericMsg with ControlMsgRspPayload.
func (b ControlMsgRspBuilder) Build() *sim.GenericMsg {
	payload := &ControlMsgRspPayload{
		Enable:  b.enable,
		Drain:   b.drain,
		Flush:   b.flush,
		Pause:   b.pause,
		Invalid: b.invalid,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			RspTo:        b.rspTo,
			TrafficClass: reflect.TypeOf(ControlMsgRspPayload{}).String(),
		},
		Payload: payload,
	}
}
