package tlb

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// A FlushReq asks the TLB to invalidate certain entries. It will also not block
// all incoming and outgoing ports
type FlushReq struct {
	modeling.MsgMeta
	VAddrs []uint64
	PID    vm.PID
}

// Meta returns the meta data associated with the message.
func (r FlushReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned FlushReq with different ID
func (r FlushReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp generates response to original flush request
func (r FlushReq) GenerateRsp() modeling.Rsp {
	rsp := modeling.GeneralRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: r.Dst,
			Dst: r.Src,
		},

		OriginalReq: r,
	}

	return rsp
}

// A RestartReq is a request to TLB to start accepting requests and resume
// operations
type RestartReq struct {
	modeling.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r RestartReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned RestartReq with different ID
func (r RestartReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp generates response to original restart request
func (r RestartReq) GenerateRsp() modeling.Rsp {
	rsp := modeling.GeneralRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: r.Dst,
			Dst: r.Src,
		},
	}

	return rsp
}
