package memcontroltest

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Controllable is the minimal interface the contract harness requires
// from a component under test. Every ticking Akita memory agent satisfies
// it.
type Controllable interface {
	timing.Handler
	Name() string
}

// Harness bundles a built component, its Control port, and a teardown
// callback. Build functions passed to RunContract return a *Harness.
type Harness struct {
	// Comp is the component under test. RunContract drives it by handing
	// it a ticking.Event at a time, as the engine does.
	Comp Controllable

	// Sim is the simulation the component belongs to. RunContract allocates
	// the IDs of the requests and ticks it makes from it.
	Sim modeling.Simulation

	// Ctrl is the component's Control port. RunContract delivers
	// ControlReq into it via Deliver and reads ControlRsp out of it via
	// RetrieveOutgoing.
	Ctrl messaging.Port

	// IsQuiescent, if non-nil, reports whether the component currently
	// holds no in-flight work. RunContract uses it to enforce that a Drain
	// ack and a Reset both leave the component quiescent. Components for
	// which quiescence is not a meaningful/cheap query may leave it nil to
	// skip the check.
	IsQuiescent func() bool

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
//   - asserts the memcontrolprotocol.Rsp matches the expected protocol response for
//     (verb, support-state).
//
// The expected response per verb is:
//
//   - Verb supported and synchronous: memcontrolprotocol.Rsp arrives within maxTicks with
//     Success=true and matching memcontrolprotocol.Command.
//   - Verb supported and asynchronous: memcontrolprotocol.Rsp arrives within asyncMaxTicks
//     with Success=true and matching memcontrolprotocol.Command.
//   - Verb unsupported: memcontrolprotocol.Rsp arrives within maxTicks with Success=false
//     and Error=memcontrolprotocol.ErrUnsupported.
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
		cmd   memcontrolprotocol.Command
		label string
	}{
		{0, "InvalidZero"},
		{memcontrolprotocol.CmdPause, "Pause"},
		{memcontrolprotocol.CmdDrain, "Drain"},
		{memcontrolprotocol.CmdEnable, "Enable"},
		{memcontrolprotocol.CmdReset, "Reset"},
		{memcontrolprotocol.CmdInvalidate, "Invalidate"},
		{memcontrolprotocol.CmdFlush, "Flush"},
		{memcontrolprotocol.CmdHold, "Hold"},
		{memcontrolprotocol.CmdRelease, "Release"},
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

	// Conditional verbs (Invalidate, Flush) are only legal once the
	// component is paused or drained. When supported, issuing them from
	// the freshly-built (Enabled) state must be rejected with
	// memcontrolprotocol.ErrMustBePausedOrDrained rather than silently acted on.
	for _, v := range verbs {
		if !matrix.Supports(v.cmd) || !isConditionalVerb(v.cmd) {
			continue
		}

		t.Run(fmt.Sprintf("%s/%s-illegal-when-enabled", name, v.label),
			func(t *testing.T) {
				h := build()
				defer func() {
					if h.Teardown != nil {
						h.Teardown()
					}
				}()

				checkConditionalIllegalState(t, h, v.cmd)
			})
	}

	runLifecycleInvariants(t, name, build, matrix)
}

// runLifecycleInvariants checks properties that single-verb round trips
// miss: the sync universal verbs are idempotent, and Reset from the paused
// state lands the component back in a clean (quiescent) enabled state.
func runLifecycleInvariants(
	t *testing.T,
	name string,
	build BuildFunc,
	matrix VerbSupport,
) {
	t.Helper()

	if matrix.Pause {
		t.Run(name+"/idempotent-pause", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdPause)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdPause)
		})
	}

	if matrix.Enable {
		t.Run(name+"/idempotent-enable", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdEnable)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdEnable)
		})
	}

	if matrix.Reset && matrix.Pause {
		t.Run(name+"/reset-from-paused", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdPause)
			driveExpectSuccess(t, h, memcontrolprotocol.CmdReset)
			if h.IsQuiescent != nil && !h.IsQuiescent() {
				t.Errorf("component not quiescent after Reset from Paused")
			}
		})
	}
}

func teardown(h *Harness) {
	if h.Teardown != nil {
		h.Teardown()
	}
}

// driveExpectSuccess delivers one verb and asserts a Success ack returns
// within the verb's tick budget.
func driveExpectSuccess(t *testing.T, h *Harness, cmd memcontrolprotocol.Command) {
	t.Helper()

	req := newControlReq(h, cmd)
	h.Ctrl.Deliver(req)

	budget := maxTicks
	if !IsSyncVerb(cmd) {
		budget = asyncMaxTicks
	}

	rsp, ok := drainForRsp(h, budget)
	if !ok {
		t.Fatalf("no ack for %v", cmd)
	}
	if rsp.Payload.(memcontrolprotocol.Rsp).Command != cmd || !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Errorf("%v: got %+v, want Success ack", cmd, rsp)
	}
}

// isConditionalVerb reports whether the verb requires the component to be
// paused or drained before it is legal.
func isConditionalVerb(cmd memcontrolprotocol.Command) bool {
	return cmd == memcontrolprotocol.CmdInvalidate || cmd == memcontrolprotocol.CmdFlush
}

// pauseForConditionalVerb drives the component to a paused state so a
// subsequent Invalidate or Flush is legal.
func pauseForConditionalVerb(t *testing.T, h *Harness) {
	t.Helper()

	req := newControlReq(h, memcontrolprotocol.CmdPause)
	h.Ctrl.Deliver(req)

	rsp, ok := drainForRsp(h, maxTicks)
	if !ok {
		t.Fatal("no response while pausing before conditional verb")
	}
	payload := rsp.Payload.(memcontrolprotocol.Rsp)
	if payload.Command != memcontrolprotocol.CmdPause || !payload.Success {
		t.Fatalf("could not pause before conditional verb; got %+v", rsp)
	}
}

// checkConditionalIllegalState delivers a conditional verb to an Enabled
// component and asserts it is rejected per the protocol.
func checkConditionalIllegalState(
	t *testing.T,
	h *Harness,
	cmd memcontrolprotocol.Command,
) {
	t.Helper()

	req := newControlReq(h, cmd)
	h.Ctrl.Deliver(req)

	rsp, ok := drainForRsp(h, maxTicks)
	if !ok {
		t.Fatalf("no ControlRsp received for %v issued while Enabled", cmd)
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Command != cmd {
		t.Errorf("memcontrolprotocol.Rsp.Command = %v, want %v", rsp.Payload.(memcontrolprotocol.Rsp).Command, cmd)
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Errorf("memcontrolprotocol.Rsp.Success = true, want false for %v while Enabled", cmd)
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Error != memcontrolprotocol.ErrMustBePausedOrDrained {
		t.Errorf("memcontrolprotocol.Rsp.Error = %q, want %q",
			rsp.Payload.(memcontrolprotocol.Rsp).Error, memcontrolprotocol.ErrMustBePausedOrDrained)
	}
}

// checkVerb performs one verb's round trip against the harness.
func checkVerb(
	t *testing.T,
	h *Harness,
	cmd memcontrolprotocol.Command,
	supported bool,
) {
	t.Helper()

	if supported && isConditionalVerb(cmd) {
		// Invalidate and Flush are only legal once the component is
		// paused or drained, so quiesce it before driving the verb.
		pauseForConditionalVerb(t, h)
	}

	req := newControlReq(h, cmd)
	h.Ctrl.Deliver(req)

	budget := maxTicks
	if supported && !IsSyncVerb(cmd) {
		budget = asyncMaxTicks
	}

	rsp, ok := drainForRsp(h, budget)
	if !ok {
		t.Fatalf("no ControlRsp received within %d ticks for verb %v "+
			"(supported=%v)", budget, cmd, supported)
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Command != cmd {
		t.Errorf("memcontrolprotocol.Rsp.Command = %v, want %v", rsp.Payload.(memcontrolprotocol.Rsp).Command, cmd)
	}

	if rsp.RspTo != req.ID {
		t.Errorf("memcontrolprotocol.Rsp.RspTo = %d, want %d", rsp.RspTo, req.ID)
	}

	if supported {
		if !rsp.Payload.(memcontrolprotocol.Rsp).Success {
			t.Errorf("memcontrolprotocol.Rsp.Success = false (Error=%q), want true",
				rsp.Payload.(memcontrolprotocol.Rsp).Error)
			return
		}
		// Drain promises quiescence on ack; Reset returns to a freshly-
		// built (quiescent) state. Enforce it when the component supplies
		// a probe.
		if h.IsQuiescent != nil &&
			(cmd == memcontrolprotocol.CmdDrain || cmd == memcontrolprotocol.CmdReset) &&
			!h.IsQuiescent() {
			t.Errorf("%v acked but component is not quiescent", cmd)
		}
		return
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Errorf("memcontrolprotocol.Rsp.Success = true, want false for unsupported verb")
	}

	if rsp.Payload.(memcontrolprotocol.Rsp).Error != memcontrolprotocol.ErrUnsupported {
		t.Errorf("memcontrolprotocol.Rsp.Error = %q, want %q",
			rsp.Payload.(memcontrolprotocol.Rsp).Error, memcontrolprotocol.ErrUnsupported)
	}
}

// newControlReq builds a ControlReq addressed to the component's
// Control port from a fixed pseudo-source "ContractAgent".
func newControlReq(h *Harness, cmd memcontrolprotocol.Command) messaging.Msg {
	req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
		ID:  h.Sim.NewID(),
		Src: messaging.RemotePort("ContractAgent"),
		Dst: h.Ctrl.AsRemote(),
	}

	return req
}

// tick hands the component a ticking.Event at the current time, as the engine
// does on every cycle.
func (h *Harness) tick() {
	h.Comp.Handle(ticking.MakeEvent(
		h.Sim.NewID(), h.Sim.Engine().CurrentTime(), h.Comp.Name()))
}

// drainForRsp ticks the component up to budget times waiting for a
// ControlRsp to appear on the Control port's outgoing queue. It returns
// the first such memcontrolprotocol.Rsp and true, or a zero memcontrolprotocol.Rsp and false if the budget is
// exhausted.
func drainForRsp(h *Harness, budget int) (messaging.Msg, bool) {
	for range budget {
		if msg, ok := h.Ctrl.RetrieveOutgoing(); ok {
			if _, ok := msg.Payload.(memcontrolprotocol.Rsp); ok {
				return msg, true
			}
		}
		h.tick()
	}

	// One last sweep in case the final tick produced the memcontrolprotocol.Rsp.
	if msg, ok := h.Ctrl.RetrieveOutgoing(); ok {
		if _, ok := msg.Payload.(memcontrolprotocol.Rsp); ok {
			return msg, true
		}
	}
	return messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, false
}
