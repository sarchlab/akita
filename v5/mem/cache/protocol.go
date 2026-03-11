package cache

import (
	"github.com/sarchlab/akita/v5/sim"
)

// FlushReq is a flush request sent to a cache unit to request it to flush all
// the cache lines.
type FlushReq struct {
	sim.MsgMeta
	InvalidateAllCachelines bool
	DiscardInflight         bool
	PauseAfterFlushing      bool
}

// FlushRsp is a response indicating a cache flush is complete.
type FlushRsp struct {
	sim.MsgMeta
}

// RestartReq is a restart request sent to a cache unit to unpause it.
type RestartReq struct {
	sim.MsgMeta
}

// RestartRsp is a response indicating a cache restart is complete.
type RestartRsp struct {
	sim.MsgMeta
}
