package mem

import (
	"reflect"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

var accessReqByteOverhead = 12
var accessRspByteOverhead = 4
var controlMsgByteOverhead = 4

// AccessReq abstracts read and write requests that are sent to the
// cache modules or memory controllers.
type AccessReq interface {
	sim.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// A AccessRsp is a respond in the memory system.
type AccessRsp interface {
	sim.Msg
	sim.Rsp
}

// A ReadReq is a request sent to a memory controller to fetch data
type ReadReq struct {
	sim.MsgMeta

	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// Meta returns the message meta.
func (r *ReadReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned ReadReq with different ID
func (r *ReadReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GenerateRsp generate DataReadyRsp to ReadReq
func (r *ReadReq) GenerateRsp(data []byte) sim.Rsp {
	rsp := DataReadyRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		WithData(data).
		Build()

	return rsp
}

// GetByteSize returns the number of byte that the request is accessing.
func (r *ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing
func (r *ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r *ReadReq) GetPID() vm.PID {
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

// Build creates a new ReadReq
func (b ReadReqBuilder) Build() *ReadReq {
	r := &ReadReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficBytes = accessReqByteOverhead
	r.Address = b.address
	r.PID = b.pid
	r.Info = b.info
	r.AccessByteSize = b.byteSize
	r.CanWaitForCoalesce = b.canWaitForCoalesce
	r.TrafficClass = reflect.TypeOf(ReadReq{}).String()

	return r
}

// A WriteReq is a request sent to a memory controller to write data
type WriteReq struct {
	sim.MsgMeta

	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// Meta returns the meta data attached to a request.
func (r *WriteReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned WriteReq with different ID
func (r *WriteReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GenerateRsp generate WriteDoneRsp to the original WriteReq
func (r *WriteReq) GenerateRsp() sim.Rsp {
	rsp := WriteDoneRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		Build()

	return rsp
}

// GetByteSize returns the number of byte that the request is writing.
func (r *WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing
func (r *WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address
func (r *WriteReq) GetPID() vm.PID {
	return r.PID
}

// WriteReqBuilder can build read requests.
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

// Build creates a new WriteReq
func (b WriteReqBuilder) Build() *WriteReq {
	r := &WriteReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.PID = b.pid
	r.Info = b.info
	r.Address = b.address
	r.Data = b.data
	r.TrafficBytes = len(r.Data) + accessReqByteOverhead
	r.DirtyMask = b.dirtyMask
	r.CanWaitForCoalesce = b.canWaitForCoalesce
	r.TrafficClass = reflect.TypeOf(WriteReq{}).String()

	return r
}

// A DataReadyRsp is the respond sent from the lower module to the higher
// module that carries the data loaded.
type DataReadyRsp struct {
	sim.MsgMeta

	RespondTo string // The ID of the request it replies
	Data      []byte
}

// Meta returns the meta data attached to each message.
func (r *DataReadyRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned DataReadyRsp with different ID
func (r *DataReadyRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GetRspTo returns the ID if the request that the respond is responding to.
func (r *DataReadyRsp) GetRspTo() string {
	return r.RespondTo
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

// Build creates a new DataReadyRsp
func (b DataReadyRspBuilder) Build() *DataReadyRsp {
	r := &DataReadyRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficBytes = len(b.data) + accessRspByteOverhead
	r.RespondTo = b.rspTo
	r.Data = b.data
	r.TrafficClass = reflect.TypeOf(ReadReq{}).String()

	return r
}

// A WriteDoneRsp is a respond sent from the lower module to the higher module
// to mark a previous requests is completed successfully.
type WriteDoneRsp struct {
	sim.MsgMeta

	RespondTo string
}

// Meta returns the meta data associated with the message.
func (r *WriteDoneRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned WriteDoneRsp with different ID
func (r *WriteDoneRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GetRspTo returns the ID of the request that the respond is responding to.
func (r *WriteDoneRsp) GetRspTo() string {
	return r.RespondTo
}

// WriteDoneRspBuilder can build data ready responds.
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

// Build creates a new WriteDoneRsp
func (b WriteDoneRspBuilder) Build() *WriteDoneRsp {
	r := &WriteDoneRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficBytes = accessRspByteOverhead
	r.RespondTo = b.rspTo
	r.TrafficClass = reflect.TypeOf(WriteReq{}).String()

	return r
}

// ControlMsg is the commonly used message type for controlling the components
// on the memory hierarchy. It is also used for responding the original
// requester with the Done field. Drain is used to process all the requests in
// the queue when drain happens the component will not accept any new requests
// Enable enables the component work. If the enable = false, it will not process
// any requests.
type ControlMsg struct {
	sim.MsgMeta

	DiscardTransations bool
	Restart            bool
	NotifyDone         bool
	Enable             bool
	Drain              bool
	Flush              bool
	Pause              bool
	Invalid            bool
}

// Meta returns the meta data associated with the ControlMsg.
func (m *ControlMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns cloned ControlMsg with different ID
func (m *ControlMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GenerateRsp generates a GeneralRsp for ControlMsg.
func (m *ControlMsg) GenerateRsp() sim.Rsp {
	rsp := sim.GeneralRspBuilder{}.
		WithSrc(m.Dst).
		WithDst(m.Src).
		WithOriginalReq(m).
		Build()

	return rsp
}

// A ControlMsgBuilder can build control messages.
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

// Build creates a new ControlMsg.
func (b ControlMsgBuilder) Build() *ControlMsg {
	m := &ControlMsg{}
	m.ID = sim.GetIDGenerator().Generate()
	m.Src = b.src
	m.Dst = b.dst
	m.TrafficBytes = controlMsgByteOverhead

	m.Enable = b.Enable
	m.Drain = b.Drain
	m.Flush = b.Flush
	m.Invalid = b.Invalid

	m.DiscardTransations = b.discardTransactions
	m.Restart = b.restart
	m.NotifyDone = b.notifyDone
	m.Enable = b.Enable
	m.Drain = b.Drain
	m.Flush = b.Flush
	m.Pause = b.Pause
	m.Invalid = b.Invalid
	m.TrafficClass = reflect.TypeOf(ControlMsg{}).String()

	return m
}

type ControlMsgRsp struct {
	sim.MsgMeta

	RspTo   string
	Enable  bool
	Drain   bool
	Flush   bool
	Pause   bool
	Invalid bool
}

// Meta returns the meta data associated with the message.
func (r *ControlMsgRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned ControlMsgRsp with different ID
func (r *ControlMsgRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	cloneMsg.Enable = r.Enable
	cloneMsg.Drain = r.Drain
	cloneMsg.Flush = r.Flush
	cloneMsg.Pause = r.Pause
	cloneMsg.Invalid = r.Invalid
	return &cloneMsg
}

// GetRspTo returns the ID of the request that the respond is responding to.
func (r *ControlMsgRsp) GetRspTo() string {
	return r.RspTo
}

// ControlMsgRspBuilder can build control messages.
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

// Build creates a new ControlMsgRsp
func (b ControlMsgRspBuilder) Build() *ControlMsgRsp {
	r := &ControlMsgRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo
	r.Enable = b.enable
	r.Drain = b.drain
	r.Flush = b.flush
	r.Pause = b.pause
	r.Invalid = b.invalid
	r.TrafficClass = reflect.TypeOf(ControlMsgRsp{}).String()
	return r
}
