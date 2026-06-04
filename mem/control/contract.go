package control

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Controllable is the minimal interface the contract harness requires
// from a component under test. Every Akita memory agent satisfies it
// through embedding modeling.TickingComponent.
type Controllable interface {
	Tick() bool
	Name() string
}

// Harness bundles a built component, its Control port, and a teardown
// callback. Build functions passed to RunContract return a *Harness.
type Harness struct {
	// Comp is the component under test. RunContract drives it via Tick.
	Comp Controllable

	// Ctrl is the component's Control port. RunContract delivers
	// ControlReq into it via Deliver and reads ControlRsp out of it via
	// RetrieveOutgoing.
	Ctrl messaging.Port

	// Teardown is invoked once after the harness finishes with this
	// component, regardless of test outcome. Nil is acceptable.
	Teardown func()
}

// BuildFunc constructs a fresh harness. The harness is rebuilt for each
// subtest so a verb cannot observe state left behind by a previous verb.
type BuildFunc func() *Harness

// maxTicks is the upper bound on how many ticks a sync verb (including
// the unsupported-verb response) is allowed to take before the harness
// gives up. The async verbs (Drain, Flush) use asyncMaxTicks.
const (
	maxTicks      = 64
	asyncMaxTicks = 4096
)

// RunContract exercises every verb in the protocol against the
// component produced by build. It uses t.Run so per-verb failures are
// reported independently.
//
// For every verb the harness:
//
//   - rebuilds the component (so verb tests are independent),
//   - delivers a ControlReq with that verb to the component's Control
//     port,
//   - drives Tick until a ControlRsp comes back (or a tick budget is
//     exhausted),
//   - asserts the Rsp matches the expected protocol response for
//     (verb, support-state).
//
// The expected response per verb is:
//
//   - Verb supported and synchronous: Rsp arrives within maxTicks with
//     Success=true and matching Command.
//   - Verb supported and asynchronous: Rsp arrives within asyncMaxTicks
//     with Success=true and matching Command.
//   - Verb unsupported: Rsp arrives within maxTicks with Success=false
//     and Error=ErrUnsupported.
//
// State-side assertions (e.g. directory cleared after Reset, transactions
// drained after Drain) live in per-component behavior tests, not here.
// This harness only enforces the protocol surface.
func RunContract(
	t *testing.T,
	name string,
	build BuildFunc,
	matrix VerbSupport,
) {
	t.Helper()

	verbs := []struct {
		cmd   mem.ControlCommand
		label string
	}{
		{mem.CmdPause, "Pause"},
		{mem.CmdDrain, "Drain"},
		{mem.CmdEnable, "Enable"},
		{mem.CmdReset, "Reset"},
		{mem.CmdInvalidate, "Invalidate"},
		{mem.CmdFlush, "Flush"},
	}

	for _, v := range verbs {
		t.Run(fmt.Sprintf("%s/%s", name, v.label), func(t *testing.T) {
			h := build()
			defer func() {
				if h.Teardown != nil {
					h.Teardown()
				}
			}()

			checkVerb(t, h, v.cmd, matrix.Supports(v.cmd))
		})
	}
}

// checkVerb performs one verb's round trip against the harness.
func checkVerb(
	t *testing.T,
	h *Harness,
	cmd mem.ControlCommand,
	supported bool,
) {
	t.Helper()

	req := newControlReq(h.Ctrl, cmd)
	h.Ctrl.Deliver(req)

	budget := maxTicks
	if supported && !IsSyncVerb(cmd) {
		budget = asyncMaxTicks
	}

	rsp := drainForRsp(h, budget)
	if rsp == nil {
		t.Fatalf("no ControlRsp received within %d ticks for verb %v "+
			"(supported=%v)", budget, cmd, supported)
	}

	if rsp.Command != cmd {
		t.Errorf("Rsp.Command = %v, want %v", rsp.Command, cmd)
	}

	if rsp.RspTo != req.ID {
		t.Errorf("Rsp.RspTo = %d, want %d", rsp.RspTo, req.ID)
	}

	if supported {
		if !rsp.Success {
			t.Errorf("Rsp.Success = false (Error=%q), want true",
				rsp.Error)
		}
		return
	}

	if rsp.Success {
		t.Errorf("Rsp.Success = true, want false for unsupported verb")
	}

	if rsp.Error != ErrUnsupported {
		t.Errorf("Rsp.Error = %q, want %q",
			rsp.Error, ErrUnsupported)
	}
}

// newControlReq builds a ControlReq addressed to the component's
// Control port from a fixed pseudo-source "ContractAgent".
func newControlReq(
	ctrl messaging.Port,
	cmd mem.ControlCommand,
) *mem.ControlReq {
	req := &mem.ControlReq{Command: cmd}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("ContractAgent")
	req.Dst = ctrl.AsRemote()
	req.TrafficClass = "mem.ControlReq"
	return req
}

// drainForRsp ticks the component up to budget times waiting for a
// ControlRsp to appear on the Control port's outgoing queue. It returns
// the first such Rsp, or nil if the budget is exhausted.
func drainForRsp(h *Harness, budget int) *mem.ControlRsp {
	for range budget {
		if msg := h.Ctrl.RetrieveOutgoing(); msg != nil {
			if rsp, ok := msg.(*mem.ControlRsp); ok {
				return rsp
			}
		}
		h.Comp.Tick()
	}

	// One last sweep in case the final tick produced the Rsp.
	if msg := h.Ctrl.RetrieveOutgoing(); msg != nil {
		if rsp, ok := msg.(*mem.ControlRsp); ok {
			return rsp
		}
	}
	return nil
}
