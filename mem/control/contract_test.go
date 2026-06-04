package control_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// fakeComp is a minimal component that satisfies the contract harness's
// expectations. It owns one Control port and implements every verb in
// the protocol: sync verbs respond immediately, async verbs respond
// after a configurable per-component number of ticks, unsupported
// verbs respond with Success=false, Error=ErrUnsupported. The test
// suite uses it to verify the harness's own logic.
type fakeComp struct {
	hooking.HookableBase

	name     string
	matrix   control.VerbSupport
	asyncDelay int
	ports    map[string]messaging.Port

	// pending captures an in-flight async verb that owes a Rsp; sync
	// verbs are answered inside the same tick and never sit here.
	pending  *pendingReq
}

type pendingReq struct {
	cmd       mem.ControlCommand
	src       messaging.RemotePort
	id        uint64
	ticksLeft int
}

func newFakeComp(name string, matrix control.VerbSupport, asyncDelay int) *fakeComp {
	c := &fakeComp{
		name:     name,
		matrix:   matrix,
		asyncDelay: asyncDelay,
		ports:    map[string]messaging.Port{},
	}
	port := messaging.NewPort(c, 4, 4, name+".Control")
	c.AddPort("Control", port)
	conn := &noopConn{}
	conn.PlugIn(port)
	return c
}

func (c *fakeComp) Name() string                       { return c.name }
func (c *fakeComp) AddPort(name string, p messaging.Port) {
	c.ports[name] = p
	p.SetComponent(c)
}
func (c *fakeComp) GetPortByName(name string) messaging.Port { return c.ports[name] }
func (c *fakeComp) Ports() []messaging.Port {
	out := make([]messaging.Port, 0, len(c.ports))
	for _, p := range c.ports {
		out = append(out, p)
	}
	return out
}
func (c *fakeComp) NotifyRecv(_ messaging.Port)     {}
func (c *fakeComp) NotifyPortFree(_ messaging.Port) {}

func (c *fakeComp) Tick() bool {
	port := c.ports["Control"]
	made := false

	if c.pending != nil {
		c.pending.ticksLeft--
		if c.pending.ticksLeft <= 0 && port.CanSend() {
			port.Send(c.makeRsp(c.pending.cmd, c.pending.src,
				c.pending.id, true, ""))
			c.pending = nil
			made = true
		}
	}

	if c.pending == nil {
		if msg := port.PeekIncoming(); msg != nil {
			if req, ok := msg.(*mem.ControlReq); ok {
				port.RetrieveIncoming()
				made = c.handleReq(port, req) || made
			}
		}
	}

	return made
}

func (c *fakeComp) handleReq(port messaging.Port, req *mem.ControlReq) bool {
	if !c.matrix.Supports(req.Command) {
		if !port.CanSend() {
			return false
		}
		port.Send(c.makeRsp(req.Command, req.Src, req.ID, false,
			control.ErrUnsupported))
		return true
	}

	if control.IsSyncVerb(req.Command) {
		if !port.CanSend() {
			return false
		}
		port.Send(c.makeRsp(req.Command, req.Src, req.ID, true, ""))
		return true
	}

	c.pending = &pendingReq{
		cmd:       req.Command,
		src:       req.Src,
		id:        req.ID,
		ticksLeft: c.asyncDelay,
	}
	return true
}

func (c *fakeComp) makeRsp(
	cmd mem.ControlCommand,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) *mem.ControlRsp {
	port := c.ports["Control"]
	rsp := &mem.ControlRsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "mem.ControlRsp"
	return rsp
}

// noopConn satisfies messaging.Connection so the fake's port has
// somewhere to send notifications. The contract harness drives the
// port via Deliver/RetrieveOutgoing, not through the connection, so
// notifying nothing is fine.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "noopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// buildFake produces a BuildFunc that constructs a fresh fakeComp for
// each verb subtest.
func buildFake(matrix control.VerbSupport, asyncDelay int) control.BuildFunc {
	return func() *control.Harness {
		c := newFakeComp("Fake", matrix, asyncDelay)
		return &control.Harness{
			Comp: c,
			Ctrl: c.GetPortByName("Control"),
		}
	}
}

func TestRunContract_AllSupported(t *testing.T) {
	matrix := control.CacheLike()
	control.RunContract(t, "all-supported", buildFake(matrix, 1), matrix)
}

func TestRunContract_OnlyUniversal(t *testing.T) {
	matrix := control.Universal()
	control.RunContract(t, "universal-only", buildFake(matrix, 1), matrix)
}

func TestRunContract_AsyncBudget(t *testing.T) {
	// Async verbs should be allowed many ticks, but the budget is
	// finite. A delay just under asyncMaxTicks must still pass.
	matrix := control.CacheLike()
	// 100 < asyncMaxTicks (4096). Comfortably within budget but well
	// past the sync budget of 64.
	control.RunContract(t, "slow-async", buildFake(matrix, 100), matrix)
}

func TestRunContract_NothingSupported(t *testing.T) {
	matrix := control.VerbSupport{}
	control.RunContract(t, "nothing-supported", buildFake(matrix, 1), matrix)
}

func TestVerbSupport_Supports(t *testing.T) {
	v := control.VerbSupport{Pause: true, Drain: true, Enable: true, Reset: true}
	cases := []struct {
		cmd  mem.ControlCommand
		want bool
	}{
		{mem.CmdPause, true},
		{mem.CmdDrain, true},
		{mem.CmdEnable, true},
		{mem.CmdReset, true},
		{mem.CmdInvalidate, false},
		{mem.CmdFlush, false},
	}
	for _, tc := range cases {
		if got := v.Supports(tc.cmd); got != tc.want {
			t.Errorf("Supports(%v) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestIsSyncVerb(t *testing.T) {
	cases := []struct {
		cmd  mem.ControlCommand
		sync bool
	}{
		{mem.CmdPause, true},
		{mem.CmdEnable, true},
		{mem.CmdReset, true},
		{mem.CmdInvalidate, true},
		{mem.CmdDrain, false},
		{mem.CmdFlush, false},
	}
	for _, tc := range cases {
		if got := control.IsSyncVerb(tc.cmd); got != tc.sync {
			t.Errorf("IsSyncVerb(%v) = %v, want %v", tc.cmd, got, tc.sync)
		}
	}
}

func TestState_String(t *testing.T) {
	cases := []struct {
		s    control.State
		want string
	}{
		{control.StateEnabled, "enabled"},
		{control.StatePausing, "pausing"},
		{control.StatePaused, "paused"},
		{control.StateDraining, "draining"},
		{control.StateFlushing, "flushing"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
