package pagetablemmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// A PID tag permits teardown without degrading hints for other address spaces.
// Each SRAM bucket holds one fingerprint. Two candidate buckets are read together.
type fingerprint struct {
	PID vm.PID
	Tag uint16
}

// SplitMix64's finalizer disperses sequential VPNs and low-valued PIDs.
func mix(x uint64) uint64 {
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func (s *state) alternate(bucket uint64, f fingerprint) uint64 {
	return bucket ^ ((mix(uint64(f.Tag)) | 1) & uint64(len(s.Filter)-1))
}

func (s *state) candidates(pid vm.PID, vpn uint64) (fingerprint, uint64, uint64) {
	h := mix(vpn ^ mix(uint64(pid)+1))
	tag := uint16(h >> 48)
	if tag == 0 {
		tag = 1
	}
	f := fingerprint{PID: pid, Tag: tag}
	first := h & uint64(len(s.Filter)-1)
	return f, first, s.alternate(first, f)
}

func (s *state) filterContains(pid vm.PID, vpn uint64) bool {
	f, a, b := s.candidates(pid, vpn)
	return s.Filter[a] == f || s.Filter[b] == f
}

// A lookup samples the SRAM on issue and exposes its answer at the due edge.
func (m *walkMiddleware) probeFilter(w *walk, now timing.VTimeInPicoSec, width *int) bool {
	if w.Stage == filterReady {
		if *width == 0 {
			return false
		}
		*width--
		w.Stage = filterPending
		w.startLookupTask(m.c, ".filter_lookup")
		w.Due = m.due(now, m.c.Spec.FilterLatency)
		w.FilterHit = m.c.State.filterContains(w.PID, w.VPN)
		return true
	}
	if now < w.Due {
		return false
	}
	w.finishLookupTask(m.c, ".filter_lookup")
	w.Stage = walkDone
	if w.FilterHit {
		w.Stage = lookupReady
	}
	return true
}

// Updating and probing share the SRAM issue width. The serialized updater gets
// one access first, then lookups use the remaining slots. Each relocation takes
// another timed access. Discarding an unplaced victim is a permitted false negative.
func (m *walkMiddleware) maintainFilter(now timing.VTimeInPicoSec, remaining *int) bool {
	c := m.c
	u := &c.State.Update
	if u.Stage == filterDelay {
		if now < u.Due {
			return false
		}
		u.Stage = ack
		if u.MoreFilter {
			u.Stage = filterUpdate
		}
		return true
	}
	if u.Stage != filterUpdate {
		return false
	}
	*remaining--
	u.MoreFilter = false
	switch u.Request.Command {
	case pt.Map, pt.Remap:
		u.MoreFilter = c.State.insertHint(u, c.Spec)
	case pt.Unmap:
		c.State.removeHint(u.Request.PID, u.Request.VAddr>>c.Spec.Log2PageSize)
	case pt.Teardown:
		if c.State.Filter[u.FilterCursor].PID == u.Request.PID {
			c.State.Filter[u.FilterCursor] = fingerprint{}
		}
		u.FilterCursor++
		u.MoreFilter = u.FilterCursor < uint64(len(c.State.Filter))
	}
	u.Due = m.due(now, c.Spec.FilterLatency)
	u.Stage = filterDelay
	return true
}

func (s *state) insertHint(u *update, spec Spec) bool {
	f, a, b := s.candidates(u.Request.PID, u.Request.VAddr>>spec.Log2PageSize)
	if u.Displaced.Tag != 0 {
		f, a, b = u.Displaced, u.Alternate, u.Alternate
	}
	if s.Filter[a] == f || s.Filter[b] == f {
		return false
	}
	if s.Filter[a].Tag == 0 {
		s.Filter[a] = f
		return false
	}
	if s.Filter[b].Tag == 0 {
		s.Filter[b] = f
		return false
	}
	if u.Kicks >= spec.FilterMaxKicks {
		return false
	}
	victim := s.Filter[a]
	s.Filter[a] = f
	u.Displaced, u.Alternate = victim, s.alternate(a, victim)
	u.Kicks++
	return true
}

func (s *state) removeHint(pid vm.PID, vpn uint64) {
	f, a, b := s.candidates(pid, vpn)
	if s.Filter[a] == f {
		s.Filter[a] = fingerprint{}
	}
	if s.Filter[b] == f {
		s.Filter[b] = fingerprint{}
	}
}
