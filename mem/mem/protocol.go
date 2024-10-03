package mem

import (
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
	src, dst           sim.Port
	pid                vm.PID
	address, byteSize  uint64
	canWaitForCoalesce bool
	info               interface{}
}

// WithSrc sets the source of the request to build.
func (b ReadReqBuilder) WithSrc(src sim.Port) ReadReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b ReadReqBuilder) WithDst(dst sim.Port) ReadReqBuilder {
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
	src, dst           sim.Port
	pid                vm.PID
	info               interface{}
	address            uint64
	data               []byte
	dirtyMask          []bool
	canWaitForCoalesce bool
}

// WithSrc sets the source of the request to build.
func (b WriteReqBuilder) WithSrc(src sim.Port) WriteReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b WriteReqBuilder) WithDst(dst sim.Port) WriteReqBuilder {
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
	src, dst sim.Port
	rspTo    string
	data     []byte
}

// WithSrc sets the source of the request to build.
func (b DataReadyRspBuilder) WithSrc(src sim.Port) DataReadyRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataReadyRspBuilder) WithDst(dst sim.Port) DataReadyRspBuilder {
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
	src, dst sim.Port
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b WriteDoneRspBuilder) WithSrc(src sim.Port) WriteDoneRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b WriteDoneRspBuilder) WithDst(dst sim.Port) WriteDoneRspBuilder {
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

	Enable  bool
	Drain   bool
	Flush   bool
	Invalid bool
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

// GenerateRsp generate GeneralRsp to ControlMsg
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
	src, dst sim.Port
	Enable   bool
	Drain    bool
	Flush    bool
	Invalid  bool
}

// WithSrc sets the source of the request to build.
func (b ControlMsgBuilder) WithSrc(src sim.Port) ControlMsgBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b ControlMsgBuilder) WithDst(dst sim.Port) ControlMsgBuilder {
	b.dst = dst
	return b
}

func (b ControlMsgBuilder) WithCtrlInfo(
	enableFlag bool,
	drainFlag bool,
	flushFlag bool,
	invalidFlag bool,
) ControlMsgBuilder {
	b.Enable = enableFlag
	b.Drain = drainFlag
	b.Flush = flushFlag
	b.Invalid = invalidFlag
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

	return m
}

// GL0InvalidateReq is a request that invalidates the L0 cache.
type GL0InvalidateReq struct {
	sim.MsgMeta
	PID vm.PID
}

// Meta returns the meta data associated with the message.
func (r *GL0InvalidateReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned GL0InvalidateReq with different ID
func (r *GL0InvalidateReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

func (r *GL0InvalidateReq) GenerateRsp(pid vm.PID) sim.Rsp {
	rsp := GL0InvalidateRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		WithPID(pid).
		Build()

	return rsp
}

// GetByteSize returns the number of byte that the request is accessing.
func (r *GL0InvalidateReq) GetByteSize() uint64 {
	return 0
}

// GetAddress returns the address that the request is accessing
func (r *GL0InvalidateReq) GetAddress() uint64 {
	return 0
}

// GetPID returns the process ID that the request is working on.
func (r *GL0InvalidateReq) GetPID() vm.PID {
	return r.PID
}

// GL0InvalidateReqBuilder can build new GL0InvalidReq.
type GL0InvalidateReqBuilder struct {
	src, dst sim.Port
	PID      vm.PID
}

// WithSrc sets the source of the request to build.
func (b GL0InvalidateReqBuilder) WithSrc(src sim.Port) GL0InvalidateReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b GL0InvalidateReqBuilder) WithDst(dst sim.Port) GL0InvalidateReqBuilder {
	b.dst = dst
	return b
}

// WithPID sets the PID of the request to build.
func (b GL0InvalidateReqBuilder) WithPID(pid vm.PID) GL0InvalidateReqBuilder {
	b.PID = pid
	return b
}

// Build creates a new GL0InvalidateReq
func (b GL0InvalidateReqBuilder) Build() *GL0InvalidateReq {
	r := &GL0InvalidateReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	return r
}

// GL0InvalidateRsp is a response to a GL0InvalidateReq.
type GL0InvalidateRsp struct {
	sim.MsgMeta
	PID       vm.PID
	RespondTo string
}

// Meta returns the meta data associated with the message.
func (r *GL0InvalidateRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned GL0InvalidateRsp with different ID
func (r *GL0InvalidateRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GetByteSize returns the number of byte that the request is accessing.
func (r *GL0InvalidateRsp) GetByteSize() uint64 {
	return 0
}

// GetAddress returns the address that the request is accessing
func (r *GL0InvalidateRsp) GetAddress() uint64 {
	return 0
}

// GetPID returns the process ID that the request is working on.
func (r *GL0InvalidateRsp) GetPID() vm.PID {
	return r.PID
}

// GetRspTo returns the ID of the request that this response is responding to.
func (r *GL0InvalidateRsp) GetRspTo() string {
	return r.RespondTo
}

// GL0InvalidateRspBuilder can build new GL0 Invalid Rsp Builder
type GL0InvalidateRspBuilder struct {
	src, dst sim.Port
	PID      vm.PID
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b GL0InvalidateRspBuilder) WithSrc(src sim.Port) GL0InvalidateRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b GL0InvalidateRspBuilder) WithDst(dst sim.Port) GL0InvalidateRspBuilder {
	b.dst = dst
	return b
}

// WithPID sets the PID of the request to build.
func (b GL0InvalidateRspBuilder) WithPID(pid vm.PID) GL0InvalidateRspBuilder {
	b.PID = pid
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b GL0InvalidateRspBuilder) WithRspTo(id string) GL0InvalidateRspBuilder {
	b.rspTo = id
	return b
}

// GetRespondTo returns the ID if the request that the respond is responding to.
func (r *GL0InvalidateRsp) GetRespondTo() string {
	return r.RespondTo
}

// Build creates a new CUPipelineRestartReq
func (b GL0InvalidateRspBuilder) Build() *GL0InvalidateRsp {
	r := &GL0InvalidateRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RespondTo = b.rspTo
	return r
}
