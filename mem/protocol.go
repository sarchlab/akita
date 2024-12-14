package mem

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// AccessReq abstracts read and write requests that are sent to the
// cache modules or memory controllers.
type AccessReq interface {
	modeling.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// A AccessRsp is a respond in the memory system.
type AccessRsp interface {
	modeling.Msg
	modeling.Rsp
}

// A ReadReq is a request sent to a memory controller to fetch data
type ReadReq struct {
	modeling.MsgMeta

	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// Meta returns the message meta.
func (r ReadReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned ReadReq with different ID
func (r ReadReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp generate DataReadyRsp to ReadReq
func (r ReadReq) GenerateRsp() modeling.Rsp {
	rsp := DataReadyRsp{
		MsgMeta: modeling.MsgMeta{
			Src: r.Dst,
			Dst: r.Src,
			ID:  id.Generate(),
		},
		RespondTo: r.ID,
	}

	return rsp
}

// GetByteSize returns the number of byte that the request is accessing.
func (r ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing
func (r ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r ReadReq) GetPID() vm.PID {
	return r.PID
}

// A WriteReq is a request sent to a memory controller to write data
type WriteReq struct {
	modeling.MsgMeta

	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{}
}

// Meta returns the meta data attached to a request.
func (r WriteReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned WriteReq with different ID
func (r WriteReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp generate WriteDoneRsp to the original WriteReq
func (r WriteReq) GenerateRsp() modeling.Rsp {
	rsp := WriteDoneRsp{
		MsgMeta: modeling.MsgMeta{
			Src: r.Dst,
			Dst: r.Src,
			ID:  id.Generate(),
		},
		RespondTo: r.ID,
	}

	return rsp
}

// GetByteSize returns the number of byte that the request is writing.
func (r WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing
func (r WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address
func (r WriteReq) GetPID() vm.PID {
	return r.PID
}

// A DataReadyRsp is the respond sent from the lower module to the higher
// module that carries the data loaded.
type DataReadyRsp struct {
	modeling.MsgMeta

	RespondTo string // The ID of the request it replies
	Data      []byte
}

// Meta returns the meta data attached to each message.
func (r DataReadyRsp) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned DataReadyRsp with different ID
func (r DataReadyRsp) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GetRspTo returns the ID if the request that the respond is responding to.
func (r DataReadyRsp) GetRspTo() string {
	return r.RespondTo
}

// A WriteDoneRsp is a respond sent from the lower module to the higher module
// to mark a previous requests is completed successfully.
type WriteDoneRsp struct {
	modeling.MsgMeta

	RespondTo string
}

// Meta returns the meta data associated with the message.
func (r WriteDoneRsp) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned WriteDoneRsp with different ID
func (r WriteDoneRsp) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GetRspTo returns the ID of the request that the respond is responding to.
func (r WriteDoneRsp) GetRspTo() string {
	return r.RespondTo
}

// ControlMsg is the commonly used message type for controlling the components
// on the memory hierarchy. It is also used for resonpding the original
// requester with the Done field.
type ControlMsg struct {
	modeling.MsgMeta

	DiscardTransactions bool
	Restart             bool
	NotifyDone          bool
}

// Meta returns the meta data assocated with the ControlMsg.
func (m ControlMsg) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

// Clone returns cloned ControlMsg with different ID
func (m ControlMsg) Clone() modeling.Msg {
	cloneMsg := m
	cloneMsg.ID = id.Generate()

	return cloneMsg
}
