package simplebankedmemory

import "github.com/sarchlab/akita/v5/sim/timing"

// wakeMW asks for a tick at the next cycle a bank can do work, so the memory
// sleeps while requests traverse its pipelines. It runs last, after this tick's
// completions and admissions.
//
// A lane head that becomes ready later still needs a tick to commit, even while
// another lane's response waits. It schedules nothing for work only a port can
// unblock: a committed response waiting on a full Top port resumes on
// NotifyPortFree, and a new request or control command arrives through
// NotifyRecv.
type wakeMW struct {
	comp *Comp
}

// Handle never reports progress: asking for a later tick is not work.
func (m *wakeMW) Handle(e timing.Event) bool {
	state := &m.comp.State
	if state.ControlState == controlStatePaused {
		return false
	}

	spec := m.comp.Spec
	now := spec.Freq.Cycle(e.Time())
	next, found := uint64(0), false
	consider := func(cycle uint64) {
		if cycle > now && (!found || cycle < next) {
			next, found = cycle, true
		}
	}

	for i := range state.Banks {
		if due, ok := state.Banks[i].Pipeline.NextReadyCycle(); ok {
			consider(due)
		}
	}

	// Dispatch stops at the first request its bank cannot accept, so only that
	// bank's admission can unblock the input queue. A full bank frees up when
	// it completes a request, which the loop above covers.
	if state.ControlState == controlStateEnabled {
		if msg, ok := m.comp.Ports.Top.PeekIncoming(); ok {
			bank := &state.Banks[requestBank(spec, msg)]
			if due, ok := bank.Pipeline.NextAcceptCycle(); ok {
				consider(due)
			}
		}
	}

	if found {
		m.comp.TickAt(cycleEdge(spec, next))
	}

	return false
}
