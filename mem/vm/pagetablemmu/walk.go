package pagetablemmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type walkMiddleware struct{ c *Comp }

func (m *walkMiddleware) Handle(e timing.Event) bool {
	progress := m.receiveDecision()
	progress = m.respond() || progress
	progress = m.advance(e.Time()) || progress
	for range m.c.Spec.AdmissionWidth {
		prefer := m.c.State.PreferFaultIn
		if m.admit(prefer) {
			m.c.State.PreferFaultIn = !prefer
		} else if !m.admit(!prefer) {
			break
		}
		progress = true
	}
	return progress
}

func (m *walkMiddleware) admit(faultIn bool) bool {
	c := m.c
	port := c.Ports.Top
	if faultIn {
		port = c.Ports.FaultIn
	}
	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}
	var pid vm.PID
	var va, device uint64
	var noMigrate bool
	if faultIn {
		req := msg.Payload.(pagefaultprotocol.Req)
		pid, va, device, noMigrate = req.PID, req.VAddr, req.DeviceID, req.NoMigrate
	} else {
		req := msg.Payload.(addrtransprotocol.Req)
		pid, va, device, noMigrate = req.PID, req.VAddr, req.DeviceID, req.NoMigrate
	}
	vpn := va >> c.Spec.Log2PageSize
	if c.State.Update.protects(c.Spec, pid, vpn) {
		return false
	}
	w := m.entry(pid, vpn)
	if w == nil || len(w.Waiters) >= c.Spec.MaxWaitersPerEntry {
		return false
	}
	port.RetrieveIncoming()
	if w.Stage == free {
		*w = walk{Stage: lookupReady, PID: pid, VPN: vpn}
		if len(c.State.Filter) > 0 {
			w.Stage = filterReady
		}
		if !c.Spec.validVA(va) {
			w.Stage = walkDone
		}
	}
	w.Waiters = append(w.Waiters, waiter{
		ID: msg.ID, Src: msg.Src, VA: va, Device: device, FaultIn: faultIn,
		NoMigrate:    noMigrate,
		requestTrace: startRequest(c, msg),
	})
	return true
}

func (m *walkMiddleware) entry(pid vm.PID, vpn uint64) *walk {
	var empty *walk
	for i := range m.c.State.Walks {
		w := &m.c.State.Walks[i]
		if w.Stage == free {
			if empty == nil {
				empty = w
			}
			continue
		}
		if w.PID == pid && w.VPN == vpn {
			return w
		}
	}
	return empty
}

func (m *walkMiddleware) advance(now timing.VTimeInPicoSec) bool {
	c := m.c
	lookups, filters := c.Spec.LookupWidth, c.Spec.FilterWidth
	progress := m.maintainFilter(now, &filters)
	var fills [maxLevels - 1]int
	nextStart := c.State.NextPipelineWalk
	for n := range c.State.Walks {
		i := (c.State.NextPipelineWalk + n) % len(c.State.Walks)
		w := &c.State.Walks[i]
		switch w.Stage {
		case filterReady, filterPending:
			if !m.probeFilter(w, now, &filters) {
				continue
			}
		case lookupReady:
			if lookups == 0 {
				continue
			}
			lookups--
			m.startLookup(w, now)
		case lookupPending:
			if now < w.Due {
				continue
			}
			w.finishLookupTask(c, ".prefix_lookup")
			w.Stage = readReady
		case fillReady:
			level := w.Level - 1
			if fills[level] >= c.Spec.FillWidth {
				continue
			}
			fills[level]++
			w.markWaiters(c, tasks.MilestoneKindHardwareResource, "prefix fill")
			c.State.Banks[level].fill(w.PID, c.Spec.prefix(w.VPN, level), w.Table, c.Spec.Banks[level].NumWays)
			w.Stage = readReady
		default:
			continue
		}
		if nextStart == c.State.NextPipelineWalk {
			nextStart = (i + 1) % len(c.State.Walks)
		}
		progress = true
	}
	c.State.NextPipelineWalk = nextStart
	return progress
}

func (m *walkMiddleware) due(now timing.VTimeInPicoSec, latency int) timing.VTimeInPicoSec {
	at := m.c.Spec.Freq.NCyclesLater(latency, now)
	m.c.TickAt(at)
	return at
}

func (m *walkMiddleware) startLookup(w *walk, now timing.VTimeInPicoSec) {
	c := m.c
	root, found := c.State.Roots[w.PID]
	if !found {
		w.Stage = walkDone
		return
	}
	w.Table = root
	w.startLookupTask(c, ".prefix_lookup")
	for level, bank := range c.State.Banks {
		if table, ok := bank.lookup(w.PID, c.Spec.prefix(w.VPN, level)); ok {
			w.Level, w.Table = level+1, table
		}
	}
	w.Stage = lookupPending
	w.Due = m.due(now, c.Spec.LookupLatency)
}
