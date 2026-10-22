package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// testMigrationHook holds every access while blocked and, once unblocked,
// remaps the accesses to remap (when set).
type testMigrationHook struct {
	blocked   bool
	remap     *vm.Page
	wakers    []Waker
	issued    []uint64
	completed []uint64
}

func (h *testMigrationHook) Resolve(
	waker Waker,
	_ vm.PID,
	_ uint64,
	page vm.Page,
) (vm.Page, bool) {
	if h.blocked {
		h.wakers = append(h.wakers, waker)
		return page, false
	}

	if h.remap != nil {
		return *h.remap, true
	}

	return page, true
}

func (h *testMigrationHook) Issued(_ vm.PID, _, pAddr uint64) {
	h.issued = append(h.issued, pAddr)
}

func (h *testMigrationHook) Completed(pAddr uint64) {
	h.completed = append(h.completed, pAddr)
}

type migrationHookRig struct {
	sim  *testsim.Simulation
	at   *Comp
	hook *testMigrationHook
}

func newMigrationHookRig(t *testing.T) *migrationHookRig {
	t.Helper()

	r := &migrationHookRig{
		sim:  testsim.New(timing.NewSerialEngine(), false),
		hook: &testMigrationHook{blocked: true},
	}

	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"MemPort"}
	spec.Translation.Destinations = []messaging.RemotePort{"TranslationProvider"}

	r.at = modeling.MakeBuilder[*Comp](spec).
		WithSimulation(r.sim).
		WithResources(Resources{MigrationHook: r.hook}).
		Build("AddressTranslator")

	ports := makePorts(r.sim, "AddressTranslator", topBufSize)
	r.at.BindPort("Top", ports.Top)
	r.at.BindPort("Bottom", ports.Bottom)
	r.at.BindPort("Translation", ports.Translation)
	r.at.BindPort("Control", ports.Control)

	for _, p := range []messaging.Port{
		ports.Top, ports.Bottom, ports.Translation, ports.Control,
	} {
		(&noopConn{}).BindPort(p)
	}

	require.NoError(t, r.sim.Initialize())

	return r
}

// translate delivers a read of vAddr and answers its translation with page.
func (r *migrationHookRig) translate(t *testing.T, vAddr uint64, page vm.Page) {
	t.Helper()

	r.at.Ports.Top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: vAddr, AccessByteSize: 4, PID: 1},
		ID:      r.sim.NewID(),
		Src:     "Agent",
		Dst:     r.at.Ports.Top.AsRemote(),
	})
	modelingtest.Tick(r.at)

	transReq, ok := r.at.Ports.Translation.RetrieveOutgoing()
	require.True(t, ok)

	r.at.Ports.Translation.Deliver(messaging.Msg{
		Payload: vmprotocol.TranslationRsp{Page: page},
		ID:      r.sim.NewID(),
		RspTo:   transReq.ID,
	})
	modelingtest.Tick(r.at)
}

func (r *migrationHookRig) respond(sent messaging.Msg, data []byte) {
	r.at.Ports.Bottom.Deliver(messaging.Msg{
		Payload: memprotocol.DataReadyRsp{Data: data},
		ID:      r.sim.NewID(),
		RspTo:   sent.ID,
	})
	modelingtest.Tick(r.at)
}

func (r *migrationHookRig) control(cmd memcontrolprotocol.Command) messaging.Msg {
	req := messaging.Msg{
		Payload: memcontrolprotocol.Req{Command: cmd},
		ID:      r.sim.NewID(),
		Src:     "Ctrl",
		Dst:     r.at.Ports.Control.AsRemote(),
	}
	r.at.Ports.Control.Deliver(req)

	return req
}

var (
	oldPage = vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x1000, Valid: true}
	newPage = vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x2000, Valid: true}
)

func TestMigrationHookHoldsAndRemaps(t *testing.T) {
	r := newMigrationHookRig(t)
	r.translate(t, 0x10040, oldPage)

	_, sent := r.at.Ports.Bottom.RetrieveOutgoing()
	require.False(t, sent)
	require.Len(t, r.at.State.HeldReqs, 1)
	require.Equal(t, uint64(1), r.at.State.NumHeldReqs)
	require.Empty(t, r.at.State.Transactions)
	require.Empty(t, r.hook.issued)
	require.Contains(t, r.hook.wakers, Waker(r.at))

	// Still blocked: ticking again does not release it.
	modelingtest.Tick(r.at)
	_, sent = r.at.Ports.Bottom.RetrieveOutgoing()
	require.False(t, sent)

	r.hook.blocked = false
	r.hook.remap = &newPage
	modelingtest.Tick(r.at)

	out, sent := r.at.Ports.Bottom.RetrieveOutgoing()
	require.True(t, sent)
	require.Equal(t, uint64(0x2040), out.Payload.(memprotocol.ReadReq).Address)
	require.Empty(t, r.at.State.HeldReqs)
	require.Equal(t, []uint64{0x2040}, r.hook.issued)

	r.respond(out, []byte{7, 7, 7, 7})

	rsp, ok := r.at.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, []byte{7, 7, 7, 7}, rsp.Payload.(memprotocol.DataReadyRsp).Data)
	require.Equal(t, []uint64{0x2040}, r.hook.completed)
}

func TestMigrationHookReportsAccessesItDoesNotHold(t *testing.T) {
	r := newMigrationHookRig(t)
	r.hook.blocked = false
	r.translate(t, 0x10040, oldPage)

	out, sent := r.at.Ports.Bottom.RetrieveOutgoing()
	require.True(t, sent)
	require.Equal(t, uint64(0x1040), out.Payload.(memprotocol.ReadReq).Address)
	require.Zero(t, r.at.State.NumHeldReqs)
	require.Equal(t, []uint64{0x1040}, r.hook.issued)

	r.respond(out, []byte{1, 2, 3, 4})
	require.Equal(t, []uint64{0x1040}, r.hook.completed)
}

func TestMigrationHookDrainWaitsForHeldAccesses(t *testing.T) {
	r := newMigrationHookRig(t)
	r.translate(t, 0x10040, oldPage)
	r.control(memcontrolprotocol.CmdDrain)

	for range 4 {
		modelingtest.Tick(r.at)
	}

	_, acked := r.at.Ports.Control.RetrieveOutgoing()
	require.False(t, acked)

	r.hook.blocked = false
	modelingtest.Tick(r.at)

	_, sent := r.at.Ports.Bottom.RetrieveOutgoing()
	require.True(t, sent)
	require.Empty(t, r.at.State.HeldReqs)
}

func TestMigrationHookResetDropsHeldAccesses(t *testing.T) {
	r := newMigrationHookRig(t)
	r.translate(t, 0x10040, oldPage)
	require.Len(t, r.at.State.HeldReqs, 1)

	r.control(memcontrolprotocol.CmdReset)
	modelingtest.Tick(r.at)

	rsp, ok := r.at.Ports.Control.RetrieveOutgoing()
	require.True(t, ok)
	require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
	require.Empty(t, r.at.State.HeldReqs)
}
