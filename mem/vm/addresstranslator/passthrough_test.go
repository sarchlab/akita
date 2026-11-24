package addresstranslator

import (
	"math"
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

func (h *holdHarness) controlPID(cmd memcontrolprotocol.Command, pid vm.PID, page uint64) {
	h.ctrl.Deliver(messaging.Msg{
		Payload: memcontrolprotocol.Req{Command: cmd, Addresses: []uint64{page}, PID: pid},
		ID:      h.sim.NewID(),
		Src:     "Driver",
		Dst:     h.ctrl.AsRemote(),
	})
}

// TestPID0HoldAndReleaseAreRejected checks that a Hold or Release of PID 0,
// whose accesses are physical and never held, is rejected at once, also while
// physical accesses to the page are in flight.
func TestPID0HoldAndReleaseAreRejected(t *testing.T) {
	h := newHoldHarness(t)

	h.physicalAccess(0x30040)
	h.tick()
	_, ok := h.bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.Len(t, h.at.State.InflightReqToBottom, 1)

	for _, cmd := range []memcontrolprotocol.Command{
		memcontrolprotocol.CmdHold, memcontrolprotocol.CmdRelease,
	} {
		h.controlPID(cmd, 0, 0x30000)
		h.tick()
		rsp, _, ok := h.ack()
		require.True(t, ok)
		require.False(t, rsp.Success)
		require.Equal(t, memcontrolprotocol.ErrInvalidPID, rsp.Error)
	}
	require.Empty(t, h.at.State.PendingHolds)
	require.Empty(t, h.at.State.HeldPages)

	// A Hold of a process does not wait for the physical accesses to the
	// same address, and does not hold the later ones.
	h.controlPID(memcontrolprotocol.CmdHold, 1, 0x30000)
	h.tick()
	rsp, _, ok := h.ack()
	require.True(t, ok)
	require.True(t, rsp.Success)

	h.physicalAccess(0x30080)
	h.tick()
	req, ok := h.bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(0x30080), req.Payload.(memprotocol.ReadReq).Address)
	require.Empty(t, h.at.State.HeldReqs)
}

func TestVirtualAccessMustNotCrossAPage(t *testing.T) {
	h := newHoldHarness(t)

	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: 0x10ffe, AccessByteSize: 4, PID: 1},
		ID:      h.sim.NewID(), Src: "Agent", Dst: h.top.AsRemote(),
	})
	require.Panics(t, func() { h.tick() })

	h = newHoldHarness(t)
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.WriteReq{Address: 0x10fc0, Data: make([]byte, 96), PID: 1},
		ID:      h.sim.NewID(), Src: "Agent", Dst: h.top.AsRemote(),
	})
	require.Panics(t, func() { h.tick() })

	// A size whose end wraps around the address space is not mistaken for
	// one that stays in the page.
	h = newHoldHarness(t)
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: 0x1002, AccessByteSize: math.MaxUint64, PID: 1},
		ID:      h.sim.NewID(), Src: "Agent", Dst: h.top.AsRemote(),
	})
	require.Panics(t, func() { h.tick() })

	// An access that ends on the last byte of the page, and a physical
	// access that crosses a page, are accepted.
	h = newHoldHarness(t)
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: 0x10ffc, AccessByteSize: 4, PID: 1},
		ID:      h.sim.NewID(), Src: "Agent", Dst: h.top.AsRemote(),
	})
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: 0x30ffe, AccessByteSize: 4},
		ID:      h.sim.NewID(), Src: "Agent", Dst: h.top.AsRemote(),
	})
	h.tick()
	_, ok := h.translationReq()
	require.True(t, ok)
	_, ok = h.bottom.RetrieveOutgoing()
	require.True(t, ok)
}

// TestBottomAlternatesBetweenPID0AndTranslatedAccesses keeps a backlog of
// physical accesses on Top while translated accesses wait for a one-slot
// Bottom port, and checks that both kinds keep going out.
func TestBottomAlternatesBetweenPID0AndTranslatedAccesses(t *testing.T) {
	h := newHoldHarnessWithSpec(t, ctrlBufSize, 1, DefaultSpec())

	const numTranslated = 3
	for i := range numTranslated {
		h.access(0x10000 + uint64(i)*4)
	}
	h.tick()
	for range numTranslated {
		req, ok := h.translationReq()
		require.True(t, ok)
		h.answer(req, false)
	}

	sent := make([]uint64, 0, 2*numTranslated)
	for range 2 * numTranslated {
		for h.top.CanDeliver() {
			h.physicalAccess(0x30000)
		}
		h.tick()
		req, ok := h.bottom.RetrieveOutgoing()
		require.True(t, ok)
		sent = append(sent, req.Payload.(memprotocol.ReadReq).Address)
	}

	// Physical (0x30000) and translated (page 0x1000) accesses take turns.
	require.Equal(t, []uint64{
		0x30000, 0x1000, 0x30000, 0x1004, 0x30000, 0x1008,
	}, sent)
}

func TestNoMigrateSpecMarksTranslationRequests(t *testing.T) {
	for _, noMigrate := range []bool{false, true} {
		spec := DefaultSpec()
		spec.NoMigrate = noMigrate
		h := newHoldHarnessWithSpec(t, ctrlBufSize, bottomBufSize, spec)

		h.access(0x10040)
		h.tick()

		req, ok := h.translationReq()
		require.True(t, ok)
		require.Equal(t, noMigrate, req.Payload.(addrtransprotocol.Req).NoMigrate)
	}
}
