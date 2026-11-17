package pagetablemmu

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/mem/vm"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type updateStage uint8

const (
	drain updateStage = iota
	zeroRoot
	probe
	zeroChild
	linkChild
	writeLeaf
	maintain
	filterUpdate
	filterDelay
	ack
)

type controlMiddleware struct{ c *Comp }

func (m *controlMiddleware) Handle(_ timing.Event) bool {
	c := m.c
	u := &c.State.Update
	if u.ID == 0 {
		msg, ok := c.Ports.Control.RetrieveIncoming()
		if !ok {
			return false
		}
		*u = update{ID: msg.ID, Src: msg.Src, Request: msg.Payload.(pt.Req), Task: c.TaskTracker().StartReqIn(c, msg)}
		if err := m.validate(u.Request); err != nil {
			u.reject(err)
		}
		return true
	}
	switch u.Stage {
	case drain:
		for _, w := range c.State.Walks {
			if w.Stage != free && u.protects(c.Spec, w.PID, w.VPN) {
				return false
			}
		}
		m.begin(u)
	case maintain:
		m.maintain(u)
	case ack:
		p := c.Ports.Control
		if !p.CanSend() {
			return false
		}
		p.Send(messaging.Msg{ID: c.NewID(), RspTo: u.ID, Src: p.AsRemote(), Dst: u.Src, Payload: pt.Rsp{Error: u.Error}})
		c.TaskTracker().EndReqIn(u.Task)
		*u = update{}
	default:
		return false
	}
	return true
}

func (u *update) reject(err error) { u.Error, u.Stage = err.Error(), ack }

func (u *update) vpnRange(spec Spec) (uint64, uint64) {
	r := u.Request
	switch r.Command {
	case pt.RegisterRoot, pt.Teardown:
		return 0, math.MaxUint64
	case pt.Invalidate:
		return r.VAddr >> spec.Log2PageSize, (r.VAddr + (r.Size - 1)) >> spec.Log2PageSize
	default:
		vpn := r.VAddr >> spec.Log2PageSize
		return vpn, vpn
	}
}

func (u *update) protects(spec Spec, pid vm.PID, vpn uint64) bool {
	if u.ID == 0 || u.Error != "" || u.Request.PID != pid {
		return false
	}
	lo, hi := u.vpnRange(spec)
	return vpn >= lo && vpn <= hi
}

func (m *controlMiddleware) validate(r pt.Req) error {
	c := m.c
	spec := c.Spec
	_, exists := c.State.Roots[r.PID]
	if r.Command == pt.RegisterRoot {
		_, owned := c.State.Owners[r.Root]
		if exists || owned || r.Root%nodeBytes != 0 || !spec.inPool(r.Root) {
			return fmt.Errorf("PID already registered or root unavailable")
		}
		return nil
	}
	if !exists {
		return fmt.Errorf("unregistered PID %d", r.PID)
	}
	if r.Command == pt.Teardown {
		return nil
	}
	if !spec.validVA(r.VAddr) || r.VAddr%spec.pageSize() != 0 {
		return fmt.Errorf("invalid page VA %#x", r.VAddr)
	}
	switch r.Command {
	case pt.Map, pt.Remap:
		return spec.validateMapping(r.Mapping)
	case pt.Unmap:
		return nil
	case pt.Invalidate:
		if r.Size == 0 || r.Size%spec.pageSize() != 0 ||
			r.Size-1 > math.MaxUint64-r.VAddr || !spec.validVA(r.VAddr+(r.Size-1)) {
			return fmt.Errorf("invalid invalidation range")
		}
		return nil
	default:
		return fmt.Errorf("unknown page-table command %d", r.Command)
	}
}

func (s Spec) validateMapping(p vm.Page) error {
	if p.PAddr%s.pageSize() != 0 || p.PAddr>>s.Log2PageSize > frameMask || p.DeviceID > math.MaxUint16 {
		return fmt.Errorf("unaligned or unrepresentable leaf mapping")
	}
	if s.inPool(p.PAddr) || s.PoolBase >= p.PAddr && s.PoolBase-p.PAddr < s.pageSize() {
		return fmt.Errorf("data mapping overlaps reserved page-table pool")
	}
	return nil
}

func (m *controlMiddleware) begin(u *update) {
	c := m.c
	r := u.Request
	switch r.Command {
	case pt.RegisterRoot:
		c.State.Owners[r.Root] = r.PID
		u.Stage = zeroRoot
	case pt.Teardown:
		delete(c.State.Roots, r.PID)
		for node, pid := range c.State.Owners {
			if pid == r.PID {
				delete(c.State.Owners, node)
			}
		}
		u.Stage = maintain
	case pt.Invalidate:
		u.Stage = maintain
	default:
		u.Table, u.Stage = c.State.Roots[r.PID], probe
	}
}

func (m *controlMiddleware) maintain(u *update) {
	c := m.c
	lo, hi := u.vpnRange(c.Spec)
	for level, bank := range c.State.Banks {
		bank.invalidate(u.Request.PID, c.Spec.prefix(lo, level), c.Spec.prefix(hi, level))
	}
	u.Stage = ack
	if len(c.State.Filter) > 0 && u.Request.Command != pt.Invalidate {
		u.Stage = filterUpdate
	}
}

// Reserve the entire missing path before making any change visible. Exhaustion
// leaves an unsuccessful map with no allocated nodes or partial pointers.
func (s *state) reserve(spec Spec, pid vm.PID, count int) ([]uint64, bool) {
	if spec.PoolNodes-uint64(len(s.Owners)) < uint64(count) {
		return nil, false
	}
	nodes := make([]uint64, 0, count)
	for len(nodes) < count {
		addr := spec.PoolBase + s.NextNode*nodeBytes
		s.NextNode = (s.NextNode + 1) % spec.PoolNodes
		if _, used := s.Owners[addr]; !used {
			nodes = append(nodes, addr)
			s.Owners[addr] = pid
		}
	}
	return nodes, true
}
