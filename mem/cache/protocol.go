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
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

func (r FlushReq) GenerateRsp() modeling.Rsp {
	rsp := modeling.GeneralRsp{
		MsgMeta: modeling.MsgMeta{
			Src: r.Dst,
			Dst: r.Src,
			ID:  id.Generate(),
		},
		OriginalReq: r,
	}

	return rsp
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
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

func (r RestartReq) GenerateRsp() modeling.Rsp {
	rsp := modeling.GeneralRsp{
		MsgMeta: modeling.MsgMeta{
			Src: r.Dst,
			Dst: r.Src,
			ID:  id.Generate(),
		},
		OriginalReq: r,
	}

	return rsp
}
