package memcontrolprotocol_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
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

	name       string
	matrix     memcontrolprotocol.VerbSupport
	asyncDelay int
	ports      map[string]messaging.Port

	// pending captures an in-flight async verb that owes a Rsp; sync
	// verbs are answered inside the same tick and never sit here.
	pending *pendingReq

	// paused tracks whether the component is paused/drained, which the
	// conditional verbs (Invalidate, Flush) require.
	paused bool
}

type pendingReq struct {
	cmd       memcontrolprotocol.Command
	src       messaging.RemotePort
	id        uint64
	ticksLeft int
}

func newFakeComp(name string, matrix memcontrolprotocol.VerbSupport, asyncDelay int) *fakeComp {
	c := &fakeComp{
		name:       name,
		matrix:     matrix,
		asyncDelay: asyncDelay,
		ports:      map[string]messaging.Port{},
	}
	port := messaging.NewPort(c, 4, 4, name+".Control")
	c.AssignPort("Control", port)
	conn := &noopConn{}
	conn.PlugIn(port)
	return c
}

func (c *fakeComp) Name() string                               { return c.name }
func (c *fakeComp) DeclarePort(_ string, _ ...*messaging.Role) {}
func (c *fakeComp) AssignPort(name string, p messaging.Port) {
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
			if c.pending.cmd == memcontrolprotocol.CmdDrain {
				c.paused = true
			}
			port.Send(c.makeRsp(c.pending.cmd, c.pending.src,
				c.pending.id, true, ""))
			c.pending = nil
			made = true
		}
	}

	if c.pending == nil {
		if msg := port.PeekIncoming(); msg != nil {
			if req, ok := msg.(memcontrolprotocol.Req); ok {
				port.RetrieveIncoming()
				made = c.handleReq(port, req) || made
			}
		}
	}

	return made
}

func (c *fakeComp) handleReq(port messaging.Port, req memcontrolprotocol.Req) bool {
	if !c.matrix.Supports(req.Command) {
		return c.respond(port, req, false, memcontrolprotocol.ErrUnsupported)
	}

	// Conditional verbs are only legal while paused or drained.
	if (req.Command == memcontrolprotocol.CmdInvalidate || req.Command == memcontrolprotocol.CmdFlush) &&
		!c.paused {
		return c.respond(port, req, false, memcontrolprotocol.ErrMustBePausedOrDrained)
	}

	switch req.Command {
	case memcontrolprotocol.CmdPause:
		c.paused = true
	case memcontrolprotocol.CmdEnable, memcontrolprotocol.CmdReset:
		c.paused = false
	}

	if memcontrolprotocol.IsSyncVerb(req.Command) {
		return c.respond(port, req, true, "")
	}

	c.pending = &pendingReq{
		cmd:       req.Command,
		src:       req.Src,
		id:        req.ID,
		ticksLeft: c.asyncDelay,
	}
	return true
}

func (c *fakeComp) respond(
	port messaging.Port,
	req memcontrolprotocol.Req,
	success bool,
	errStr string,
) bool {
	if !port.CanSend() {
		return false
	}
	port.Send(c.makeRsp(req.Command, req.Src, req.ID, success, errStr))
	return true
}

func (c *fakeComp) makeRsp(
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) memcontrolprotocol.Rsp {
	port := c.ports["Control"]
	rsp := memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "memcontrolprotocol.Rsp"
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
func buildFake(matrix memcontrolprotocol.VerbSupport, asyncDelay int) memcontrolprotocol.BuildFunc {
	return func() *memcontrolprotocol.Harness {
		c := newFakeComp("Fake", matrix, asyncDelay)
		return &memcontrolprotocol.Harness{
			Comp:        c,
			Ctrl:        c.GetPortByName("Control"),
			IsQuiescent: func() bool { return c.pending == nil },
		}
	}
}

func TestRunContract_AllSupported(t *testing.T) {
	matrix := memcontrolprotocol.CacheLike()
	memcontrolprotocol.RunContract(t, "all-supported", buildFake(matrix, 1), matrix)
}

func TestRunContract_OnlyUniversal(t *testing.T) {
	matrix := memcontrolprotocol.Universal()
	memcontrolprotocol.RunContract(t, "universal-only", buildFake(matrix, 1), matrix)
}

func TestRunContract_AsyncBudget(t *testing.T) {
	// Async verbs should be allowed many ticks, but the budget is
	// finite. A delay just under asyncMaxTicks must still pass.
	matrix := memcontrolprotocol.CacheLike()
	// 100 < asyncMaxTicks (4096). Comfortably within budget but well
	// past the sync budget of 64.
	memcontrolprotocol.RunContract(t, "slow-async", buildFake(matrix, 100), matrix)
}

func TestRunContract_NothingSupported(t *testing.T) {
	matrix := memcontrolprotocol.VerbSupport{}
	memcontrolprotocol.RunContract(t, "nothing-supported", buildFake(matrix, 1), matrix)
}

func TestVerbSupport_Supports(t *testing.T) {
	v := memcontrolprotocol.VerbSupport{Pause: true, Drain: true, Enable: true, Reset: true}
	cases := []struct {
		cmd  memcontrolprotocol.Command
		want bool
	}{
		{memcontrolprotocol.CmdPause, true},
		{memcontrolprotocol.CmdDrain, true},
		{memcontrolprotocol.CmdEnable, true},
		{memcontrolprotocol.CmdReset, true},
		{memcontrolprotocol.CmdInvalidate, false},
		{memcontrolprotocol.CmdFlush, false},
	}
	for _, tc := range cases {
		if got := v.Supports(tc.cmd); got != tc.want {
			t.Errorf("Supports(%v) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestIsSyncVerb(t *testing.T) {
	cases := []struct {
		cmd  memcontrolprotocol.Command
		sync bool
	}{
		{memcontrolprotocol.CmdPause, true},
		{memcontrolprotocol.CmdEnable, true},
		{memcontrolprotocol.CmdReset, true},
		{memcontrolprotocol.CmdInvalidate, true},
		{memcontrolprotocol.CmdDrain, false},
		{memcontrolprotocol.CmdFlush, false},
	}
	for _, tc := range cases {
		if got := memcontrolprotocol.IsSyncVerb(tc.cmd); got != tc.sync {
			t.Errorf("IsSyncVerb(%v) = %v, want %v", tc.cmd, got, tc.sync)
		}
	}
}

func TestState_String(t *testing.T) {
	cases := []struct {
		s    memcontrolprotocol.State
		want string
	}{
		{memcontrolprotocol.StateEnabled, "enabled"},
		{memcontrolprotocol.StatePausing, "pausing"},
		{memcontrolprotocol.StatePaused, "paused"},
		{memcontrolprotocol.StateDraining, "draining"},
		{memcontrolprotocol.StateFlushing, "flushing"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
