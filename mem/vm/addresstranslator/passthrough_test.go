package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func (h *holdHarness) physicalAccess(addr uint64) uint64 {
	msg := messaging.Msg{
		Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
		ID:      h.sim.NewID(),
		Src:     "Agent",
		Dst:     h.top.AsRemote(),
	}
	h.top.Deliver(msg)

	return msg.ID
}

func (h *holdHarness) answerBottom(req messaging.Msg) {
	h.bottom.Deliver(messaging.Msg{
		Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}},
		ID:      h.sim.NewID(),
		RspTo:   req.ID,
	})
}

func TestPID0AccessPassesThroughNextToTranslatedAccess(t *testing.T) {
	h := newHoldHarness(t)

	h.access(0x10040)
	physID := h.physicalAccess(0x30080)
	h.tick()

	// Only the PID-1 access asks for a translation.
	transReq, ok := h.translationReq()
	require.True(t, ok)
	require.Equal(t, vm.PID(1), transReq.Payload.(addrtransprotocol.Req).PID)
	_, ok = h.translationReq()
	require.False(t, ok)

	// The PID-0 access leaves at its own address.
	physReq, ok := h.bottom.RetrieveOutgoing()
	require.True(t, ok)
	read := physReq.Payload.(memprotocol.ReadReq)
	require.Equal(t, uint64(0x30080), read.Address)
	require.Equal(t, uint64(4), read.AccessByteSize)
	require.Equal(t, vm.PID(0), read.PID)
	require.Equal(t, messaging.RemotePort("MemPort"), physReq.Dst)

	h.answerBottom(physReq)
	h.tick()
	rsp, ok := h.top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, physID, rsp.RspTo)
	require.IsType(t, memprotocol.DataReadyRsp{}, rsp.Payload)

	// The translated access completes as before.
	h.answer(transReq, false)
	h.tick()
	virtReq, ok := h.bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(0x1040), virtReq.Payload.(memprotocol.ReadReq).Address)
	h.answerBottom(virtReq)
	h.tick()
	_, ok = h.top.RetrieveOutgoing()
	require.True(t, ok)
	require.Empty(t, h.at.State.InflightReqToBottom)
	require.Empty(t, h.at.State.Transactions)
}

func TestPID0AccessIgnoresHolds(t *testing.T) {
	h := newHoldHarness(t)

	h.ctrl.Deliver(messaging.Msg{
		Payload: memcontrolprotocol.Req{
			Command: memcontrolprotocol.CmdHold, Addresses: []uint64{0x30000},
		},
		ID:  h.sim.NewID(),
		Src: "Driver",
		Dst: h.ctrl.AsRemote(),
	})
	h.tick()
	_, _, ok := h.ack()
	require.True(t, ok)

	h.physicalAccess(0x30040)
	h.tick()

	req, ok := h.bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(0x30040), req.Payload.(memprotocol.ReadReq).Address)
	require.Empty(t, h.at.State.HeldReqs)
}

func TestNoMigrateSpecMarksTranslationRequests(t *testing.T) {
	for _, noMigrate := range []bool{false, true} {
		spec := DefaultSpec()
		spec.NoMigrate = noMigrate
		h := newHoldHarnessWithSpec(t, ctrlBufSize, spec)

		h.access(0x10040)
		h.tick()

		req, ok := h.translationReq()
		require.True(t, ok)
		require.Equal(t, noMigrate, req.Payload.(addrtransprotocol.Req).NoMigrate)
	}
}
