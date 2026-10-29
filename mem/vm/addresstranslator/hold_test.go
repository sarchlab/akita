package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// holdHarness drives one translator through its real ports.
type holdHarness struct {
	t           *testing.T
	sim         *testsim.Simulation
	at          *Comp
	top         messaging.Port
	bottom      messaging.Port
	translation messaging.Port
	ctrl        messaging.Port
}

func newHoldHarness(t *testing.T) *holdHarness {
	t.Helper()

	sim := testsim.New(timing.NewSerialEngine(), false)

	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"MemPort"}
	spec.Translation.Destinations = []messaging.RemotePort{"TranslationProvider"}

	at := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		Build("AddressTranslator")
	ports := makePorts(sim, "AddressTranslator", topBufSize)
	at.BindPort("Top", ports.Top)
	at.BindPort("Bottom", ports.Bottom)
	at.BindPort("Translation", ports.Translation)
	at.BindPort("Control", ports.Control)

	for _, p := range []messaging.Port{
		at.Ports.Top, at.Ports.Bottom, at.Ports.Translation, at.Ports.Control,
	} {
		(&noopConn{}).BindPort(p)
	}

	require.NoError(t, sim.Initialize())

	return &holdHarness{
		t:           t,
		sim:         sim,
		at:          at,
		top:         at.Ports.Top,
		bottom:      at.Ports.Bottom,
		translation: at.Ports.Translation,
		ctrl:        at.Ports.Control,
	}
}

func (h *holdHarness) tick() {
	mws := h.at.Middlewares
	mws.Ctrl.Handle(nil)
	mws.ParseTranslate.Handle(nil)
	mws.RespondPipeline.Handle(nil)
}

func (h *holdHarness) access(vAddr uint64) {
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: vAddr, AccessByteSize: 4, PID: 1},
		ID:      h.sim.NewID(),
		Src:     "Agent",
		Dst:     h.top.AsRemote(),
	})
}

func (h *holdHarness) translationReq() (messaging.Msg, bool) {
	return h.translation.RetrieveOutgoing()
}

func (h *holdHarness) answer(req messaging.Msg, fault bool) {
	h.translation.Deliver(messaging.Msg{
		Payload: addrtransprotocol.Rsp{
			Page:  vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x1000, Valid: true},
			Fault: fault,
		},
		ID:    h.sim.NewID(),
		RspTo: req.ID,
	})
}

func (h *holdHarness) control(cmd memcontrolprotocol.Command) uint64 {
	msg := messaging.Msg{
		Payload: memcontrolprotocol.Req{
			Command: cmd, Addresses: []uint64{0x10000}, PID: 1,
		},
		ID:  h.sim.NewID(),
		Src: "Driver",
		Dst: h.ctrl.AsRemote(),
	}
	h.ctrl.Deliver(msg)

	return msg.ID
}

func (h *holdHarness) ack() (memcontrolprotocol.Rsp, uint64, bool) {
	msg, ok := h.ctrl.RetrieveOutgoing()
	if !ok {
		return memcontrolprotocol.Rsp{}, 0, false
	}

	return msg.Payload.(memcontrolprotocol.Rsp), msg.RspTo, true
}

func TestHoldHoldsNewAccessesWithoutTranslating(t *testing.T) {
	h := newHoldHarness(t)

	holdID := h.control(memcontrolprotocol.CmdHold)
	h.tick()

	rsp, rspTo, ok := h.ack()
	require.True(t, ok)
	require.Equal(t, memcontrolprotocol.CmdHold, rsp.Command)
	require.Equal(t, holdID, rspTo)

	h.access(0x10040)
	h.tick()

	_, sent := h.translationReq()
	require.False(t, sent)
	require.Len(t, h.at.State.HeldReqs, 1)

	h.control(memcontrolprotocol.CmdRelease)
	h.tick()

	rsp, _, ok = h.ack()
	require.True(t, ok)
	require.Equal(t, memcontrolprotocol.CmdRelease, rsp.Command)

	h.tick()

	req, sent := h.translationReq()
	require.True(t, sent)
	require.Equal(t, uint64(0x10000), req.Payload.(addrtransprotocol.Req).VAddr)
	require.Empty(t, h.at.State.HeldReqs)
	require.Empty(t, h.at.State.ReplayReqs)
}

func TestHoldDoesNotAffectOtherPages(t *testing.T) {
	h := newHoldHarness(t)

	h.control(memcontrolprotocol.CmdHold)
	h.tick()
	h.ack()

	h.access(0x20040)
	h.tick()

	_, sent := h.translationReq()
	require.True(t, sent)
}

func TestHoldAcksAfterTheTranslationInFlightReturns(t *testing.T) {
	h := newHoldHarness(t)

	h.access(0x10040)
	h.tick()
	req, _ := h.translationReq()

	h.control(memcontrolprotocol.CmdHold)
	h.tick()
	_, _, ok := h.ack()
	require.False(t, ok)

	// The translation returns after the Hold: the access is held, not sent
	// with a translation that may be stale.
	h.answer(req, false)
	h.tick()
	_, sent := h.bottom.RetrieveOutgoing()
	require.False(t, sent)
	require.Len(t, h.at.State.HeldReqs, 1)

	h.tick()
	rsp, _, ok := h.ack()
	require.True(t, ok)
	require.Equal(t, memcontrolprotocol.CmdHold, rsp.Command)
}

func TestHoldAcksAfterTheIssuedAccessCompletes(t *testing.T) {
	h := newHoldHarness(t)

	h.access(0x10040)
	h.tick()
	req, _ := h.translationReq()
	h.answer(req, false)
	h.tick()
	read, sent := h.bottom.RetrieveOutgoing()
	require.True(t, sent)

	h.control(memcontrolprotocol.CmdHold)
	h.tick()
	h.tick()
	_, _, ok := h.ack()
	require.False(t, ok)

	h.bottom.Deliver(messaging.Msg{
		Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}},
		ID:      h.sim.NewID(),
		RspTo:   read.ID,
	})
	h.tick()
	h.tick()

	rsp, _, ok := h.ack()
	require.True(t, ok)
	require.Equal(t, memcontrolprotocol.CmdHold, rsp.Command)
}

func TestFaultHoldsTheAccessAndLaterAccessesUntilRelease(t *testing.T) {
	h := newHoldHarness(t)

	h.access(0x10040)
	h.tick()
	req, _ := h.translationReq()
	h.answer(req, true)
	h.tick()

	_, sent := h.bottom.RetrieveOutgoing()
	require.False(t, sent)
	require.Equal(t, uint64(1), h.at.State.NumFaults)
	require.Len(t, h.at.State.HeldReqs, 1)

	h.access(0x10080)
	h.tick()
	_, sent = h.translationReq()
	require.False(t, sent)
	require.Len(t, h.at.State.HeldReqs, 2)

	h.control(memcontrolprotocol.CmdRelease)
	h.tick()
	_, _, ok := h.ack()
	require.True(t, ok)

	h.tick()
	_, sent = h.translationReq()
	require.True(t, sent)
	_, sent = h.translationReq()
	require.True(t, sent)
	require.Len(t, h.at.State.Transactions, 2)
}

func TestDrainWaitsForHeldAccesses(t *testing.T) {
	h := newHoldHarness(t)

	h.control(memcontrolprotocol.CmdHold)
	h.tick()
	h.ack()

	h.access(0x10040)
	h.tick()

	h.ctrl.Deliver(messaging.Msg{
		Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdDrain},
		ID:      h.sim.NewID(),
		Src:     "Driver",
		Dst:     h.ctrl.AsRemote(),
	})
	h.tick()
	h.tick()

	_, _, ok := h.ack()
	require.False(t, ok)
}

func TestResetDropsHeldAccesses(t *testing.T) {
	h := newHoldHarness(t)

	h.control(memcontrolprotocol.CmdHold)
	h.tick()
	h.ack()

	h.access(0x10040)
	h.tick()

	h.ctrl.Deliver(messaging.Msg{
		Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:      h.sim.NewID(),
		Src:     "Driver",
		Dst:     h.ctrl.AsRemote(),
	})
	h.tick()

	require.Empty(t, h.at.State.HeldReqs)
	require.Empty(t, h.at.State.HeldPages)
}
