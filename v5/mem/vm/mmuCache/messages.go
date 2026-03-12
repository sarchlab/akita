package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// FlushReq is a mmuCache flush request to invalidate certain entries.
type FlushReq struct {
	sim.MsgMeta
	VAddr []uint64
	PID   vm.PID
}

// FlushRsp is a mmuCache flush response.
type FlushRsp struct {
	sim.MsgMeta
}

// RestartReq is a mmuCache restart request.
type RestartReq struct {
	sim.MsgMeta
}

// RestartRsp is a mmuCache restart response.
type RestartRsp struct {
	sim.MsgMeta
}
