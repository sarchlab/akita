package cache

import (
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// FlushReq is the request send to a cache unit to request it to flush all
// the cache lines.
type FlushReq struct {
	modeling.MsgMeta
	InvalidateAllCachelines bool
	DiscardInflight         bool
	PauseAfterFlushing      bool
}

// Meta returns the meta data associated with the message.
func (r FlushReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned FlushReq with different ID
func (r FlushReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

func (r *FlushReq) GenerateRsp() modeling.Rsp {
	rsp := FlushRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: r.MsgMeta.Dst,
			Dst: r.MsgMeta.Src,
		},
		RspTo: r.MsgMeta.ID,
	}

	return rsp
}

// FlushRsp is the respond sent from the a cache unit for finishing a cache
// flush
type FlushRsp struct {
	modeling.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r FlushRsp) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned FlushRsp with different ID
func (r FlushRsp) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

func (r FlushRsp) GetRspTo() string {
	return r.RspTo
}

// RestartReq is the request send to a cache unit to request it unpause the
// cache
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
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

func (r RestartReq) GenerateRsp() modeling.Rsp {
	rsp := RestartRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: r.MsgMeta.Dst,
			Dst: r.MsgMeta.Src,
		},
		RspTo: r.MsgMeta.ID,
	}

	return rsp
}

// RestartRsp is the respond sent from the a cache unit for finishing a cache
// flush
type RestartRsp struct {
	modeling.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r RestartRsp) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned RestartRsp with different ID
func (r RestartRsp) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

func (r RestartRsp) GetRspTo() string {
	return r.RspTo
}
