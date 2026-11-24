package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/stretchr/testify/require"
)

type retrySendPort struct {
	messaging.Port
	checks int
}

func (p *retrySendPort) CanSend() bool {
	p.checks++
	return p.Port.CanSend()
}

func (p *retrySendPort) fill(comp *Comp, payload any) {
	for p.Port.CanSend() {
		p.Port.Send(messaging.Msg{ID: comp.NewID(), Src: p.AsRemote(), Dst: "Peer", Payload: payload})
	}
}

type retryRig struct {
	comp                     *Comp
	top, bottom, translation *retrySendPort
}

func newRetryRig(t *testing.T, tracking bool) *retryRig {
	t.Helper()
	sim := testsim.New(testsim.NewClock(), tracking)
	spec := DefaultSpec()
	spec.Log2PageSize = 12
	spec.NumReqPerCycle = 4
	spec.Bottom.Destinations = []messaging.RemotePort{"MemPort"}
	spec.Translation.Destinations = []messaging.RemotePort{"TranslationProvider"}
	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("AddressTranslator")
	ports := makePorts(sim, "AddressTranslator", topBufSize)
	r := &retryRig{comp: comp, top: &retrySendPort{Port: ports.Top},
		bottom: &retrySendPort{Port: ports.Bottom}, translation: &retrySendPort{Port: ports.Translation}}
	comp.BindPort("Top", r.top)
	comp.BindPort("Bottom", r.bottom)
	comp.BindPort("Translation", r.translation)
	comp.BindPort("Control", ports.Control)
	for _, p := range []messaging.Port{r.top, r.bottom, r.translation, ports.Control} {
		(&noopConn{}).BindPort(p)
	}
	require.NoError(t, sim.Initialize())
	return r
}

func (r *retryRig) deliverRead(addr uint64) {
	r.top.Deliver(messaging.Msg{ID: r.comp.NewID(), Src: "Agent", Dst: r.top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4, PID: 1}})
}

func TestTranslateStopsAtBlockedPortWithoutAllocatingIDs(t *testing.T) {
	r := newRetryRig(t, false)
	r.translation.fill(r.comp, addrtransprotocol.Req{})
	r.deliverRead(0x1000)
	r.deliverRead(0x2000)
	before := r.comp.NewID()

	require.False(t, r.comp.Middlewares.ParseTranslate.Handle(nil))

	require.Equal(t, 1, r.translation.checks)
	require.Equal(t, before+1, r.comp.NewID())
	require.Equal(t, 2, r.top.IncomingBuf().Size())
}

func TestDrainingReplayStopsAtBlockedPortWithoutAllocatingIDs(t *testing.T) {
	r := newRetryRig(t, false)
	r.translation.fill(r.comp, addrtransprotocol.Req{})
	r.comp.State.ControlState = controlStateDraining
	r.comp.State.ReplayReqs = []incomingReqState{{Address: 0x1000}, {Address: 0x2000}}
	before := r.comp.NewID()

	require.False(t, r.comp.Middlewares.ParseTranslate.Handle(nil))

	require.Equal(t, 1, r.translation.checks)
	require.Equal(t, before+1, r.comp.NewID())
	require.Len(t, r.comp.State.ReplayReqs, 2)
}

func TestBlockedTranslatedSendAllocatesNoIDs(t *testing.T) {
	r := newRetryRig(t, false)
	r.bottom.fill(r.comp, memprotocol.ReadReq{})
	r.comp.State.Transactions = []transactionState{{
		TranslationReqID: 200,
		IncomingReqs:     []incomingReqState{{Type: "memprotocol.ReadReq", Address: 0x1000}},
	}}
	r.translation.Deliver(messaging.Msg{ID: r.comp.NewID(), RspTo: 200,
		Payload: addrtransprotocol.Rsp{Page: vm.Page{VAddr: 0x1000, PAddr: 0x8000, Valid: true}}})
	before := r.comp.NewID()

	require.False(t, r.comp.Middlewares.RespondPipeline.Handle(nil))

	require.Equal(t, 1, r.bottom.checks)
	require.Equal(t, before+1, r.comp.NewID())
	require.Equal(t, 1, r.translation.IncomingBuf().Size())
}

func TestBlockedResponseKeepsArrivalMilestoneAndOtherStage(t *testing.T) {
	r := newRetryRig(t, true)
	rec := &traceRecorder{}
	r.comp.TaskTracker().Subscribe(rec, r.comp)
	r.top.fill(r.comp, memprotocol.DataReadyRsp{})
	msg := messaging.Msg{ID: r.comp.NewID(), Src: "Agent", Dst: r.top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: 0x1000, AccessByteSize: 4}}
	taskID := r.comp.TaskTracker().StartReqIn(r.comp, msg)
	r.comp.State.InflightReqToBottom = []reqToBottomState{{
		RecvTaskID: taskID, ReqFromTopID: msg.ID, ReqFromTopSrc: "Agent", ReqToBottomID: 100,
	}}
	r.bottom.Deliver(messaging.Msg{ID: r.comp.NewID(), RspTo: 100,
		Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}})
	r.translation.Deliver(messaging.Msg{ID: r.comp.NewID(), RspTo: 999,
		Payload: addrtransprotocol.Rsp{}})

	require.True(t, r.comp.Middlewares.RespondPipeline.Handle(nil),
		"a stale translation response is consumed while Top is blocked")

	require.Equal(t, 1, r.top.checks)
	require.Equal(t, []tasks.MilestoneKind{tasks.MilestoneKindData}, rec.kindsOn(taskID))
	require.Equal(t, 1, r.bottom.IncomingBuf().Size())
	require.Zero(t, r.translation.IncomingBuf().Size())
}

func TestBlockedResponseAllocatesNoIDs(t *testing.T) {
	r := newRetryRig(t, false)
	r.top.fill(r.comp, memprotocol.WriteDoneRsp{})
	r.comp.State.InflightReqToBottom = []reqToBottomState{{
		ReqFromTopID: 5, ReqFromTopSrc: "Agent", ReqToBottomID: 100,
	}}
	r.bottom.Deliver(messaging.Msg{ID: r.comp.NewID(), RspTo: 100, Payload: memprotocol.WriteDoneRsp{}})
	before := r.comp.NewID()

	require.False(t, r.comp.Middlewares.RespondPipeline.Handle(nil))

	require.Equal(t, 1, r.top.checks)
	require.Equal(t, before+1, r.comp.NewID())
	require.Len(t, r.comp.State.InflightReqToBottom, 1)
}
