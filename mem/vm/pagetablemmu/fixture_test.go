package pagetablemmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	at "github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type client struct{ s *sim.Simulation }

func (p client) Name() string { return "Client" }
func (p client) BindPort(slot string, port messaging.Port) {
	port.BindOwner(p, p.Name()+"."+slot)
	p.s.RegisterPort(port)
}

func (client) NotifyRecv(messaging.Port)     {}
func (client) NotifyPortFree(messaging.Port) {}

type sent struct {
	Time timing.VTimeInPicoSec
	Msg  messaging.Msg
}

type sentLog struct {
	s     *sim.Simulation
	Items []sent
}

func (h *sentLog) Func(ctx hooking.HookCtx) {
	if ctx.Pos == messaging.HookPosPortMsgSend {
		h.Items = append(h.Items, sent{Time: h.s.Engine().CurrentTime(), Msg: ctx.Item.(messaging.Msg)})
	}
}

type rig struct {
	pending               []func()
	t                     *testing.T
	s                     *sim.Simulation
	bus                   *direct.Connection
	mmu, io               *Comp
	request, ctrl, policy messaging.Port
	storage               *mem.Storage
	traffic               sentLog
	tlb                   *tlb.Comp
}

func baseSpec() Spec {
	s := DefaultSpec()
	s.Memory.Destinations = []messaging.RemotePort{"DRAM.Top"}
	s.FaultHandler = "Client.Policy"
	s.PoolBase = 0x10000
	s.PoolNodes = 24
	return s
}

func newRig(t *testing.T, local Spec, authoritative *Spec, withTLB bool) *rig {
	t.Helper()
	return newRigWithSimulation(t, sim.MakeBuilder().Build(), local, authoritative, withTLB)
}

func newRigWithSimulation(t *testing.T, s *sim.Simulation, local Spec, authoritative *Spec, withTLB bool) *rig {
	t.Helper()
	r := &rig{t: t, s: s}
	r.s.Engine().RegisterHandler("Inject", r)
	r.bus = direct.NewConnection("Bus", r.s, timing.GHz)
	r.traffic.s = r.s
	client := client{r.s}
	r.request = r.bind(client, "Requests")
	r.ctrl = r.bind(client, "Control")
	r.policy = r.bind(client, "Policy")
	r.storage = mem.MakeStorageBuilder().WithSimulation(r.s).WithCapacity(1 << 21).Build("Storage")
	dram := idealmemcontroller.DefaultSpec()
	dram.Latency, dram.NumReqPerCycle = 3, 8
	d := modeling.MakeBuilder[*idealmemcontroller.Comp](dram).WithSimulation(r.s).
		WithResources(idealmemcontroller.Resources{Storage: r.storage}).Build("DRAM")
	r.bind(d, "Top")
	r.bind(d, "Control")
	if authoritative != nil {
		local.FaultHandler = "IO.FaultIn"
		r.io = r.buildMMU("IO", *authoritative)
	}
	r.mmu = r.buildMMU("Local", local)
	if withTLB {
		tSpec := tlb.DefaultSpec()
		tSpec.Bottom.Destinations = []messaging.RemotePort{r.mmu.Ports.Top.AsRemote()}
		tSpec.Log2PageSize = uint64(local.Log2PageSize)
		r.tlb = modeling.MakeBuilder[*tlb.Comp](tSpec).WithSimulation(r.s).Build("TLB")
		for _, slot := range []string{"Top", "Bottom", "Control"} {
			r.bind(r.tlb, slot)
		}
	}
	require.NoError(t, r.s.Initialize())
	t.Cleanup(func() { require.NoError(t, r.s.Terminate()) })
	return r
}

func (r *rig) bind(owner messaging.PortOwner, slot string) messaging.Port {
	p := twowaybuffered.MakeBuilder().WithSimulation(r.s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).Build()
	owner.BindPort(slot, p)
	r.bus.BindPort(p)
	return p
}

func (r *rig) buildMMU(name string, spec Spec) *Comp {
	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(r.s).Build(name)
	for _, slot := range []string{"Top", "Memory", "FaultIn", "PageFault", "Control"} {
		r.bind(c, slot).AcceptHook(&r.traffic)
	}
	return c
}

func (r *rig) send(p messaging.Port, dst messaging.RemotePort, payload any, rspTo uint64) uint64 {
	id := r.s.NewID()
	msg := messaging.Msg{ID: id, Src: p.AsRemote(), Dst: dst, RspTo: rspTo, Payload: payload}
	r.onNextEdge(func() { p.Send(msg) })
	return id
}

// Test clients inject and consume traffic at foreground clock edges. All
// client actions have run by the time any test takes its checkpoint.
func (r *rig) onNextEdge(action func()) {
	if len(r.pending) == 0 {
		at := timing.GHz.NextTick(r.s.Engine().CurrentTime())
		r.s.Engine().Schedule(timing.MakeEventBase(r.s.NewID(), at, "Inject"))
	}
	r.pending = append(r.pending, action)
}
func (r *rig) Handle(timing.Event) {
	for _, action := range r.pending {
		action()
	}
	r.pending = nil
}
func (r *rig) run() {
	r.t.Helper()
	require.NoError(r.t, r.s.Run())
}

func (r *rig) recv(p messaging.Port) messaging.Msg {
	r.t.Helper()
	msg, ok := p.RetrieveIncoming()
	require.True(r.t, ok, "expected message in %s", p.Name())
	return msg
}

func (r *rig) cmd(c *Comp, req pt.Req) string {
	r.t.Helper()
	id := r.send(r.ctrl, c.Ports.Control.AsRemote(), req, 0)
	r.run()
	msg := r.recv(r.ctrl)
	require.Equal(r.t, id, msg.RspTo)
	return msg.Payload.(pt.Rsp).Error
}

func (r *rig) install(c *Comp, va, pa, device uint64) {
	r.t.Helper()
	require.Empty(r.t, r.cmd(c, pt.Req{Command: pt.Map, PID: 7, VAddr: va,
		Mapping: vm.Page{PAddr: pa, DeviceID: device, IsPinned: true, Unified: true}}))
}

func (r *rig) root(c *Comp) {
	r.t.Helper()
	require.Empty(r.t, r.cmd(c, pt.Req{Command: pt.RegisterRoot, PID: 7, Root: c.Spec.PoolBase}))
}

func (r *rig) translate(va, device uint64) uint64 {
	p := r.mmu.Ports.Top
	if r.tlb != nil {
		p = r.tlb.Ports.Top
	}
	return r.send(r.request, p.AsRemote(), at.Req{PID: 7, VAddr: va, DeviceID: device}, 0)
}

func (r *rig) reads(port messaging.Port) []uint64 {
	var out []uint64
	for _, item := range r.traffic.Items {
		if p, ok := item.Msg.Payload.(memprotocol.ReadReq); ok && item.Msg.Src == port.AsRemote() {
			out = append(out, p.Address)
		}
	}
	return out
}
