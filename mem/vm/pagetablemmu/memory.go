package pagetablemmu

import (
	"encoding/binary"
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type memoryMiddleware struct{ c *Comp }

const (
	maxZeroWrites           = 8
	defaultMaxZeroWriteSize = 64
)

func (m *memoryMiddleware) Handle(_ timing.Event) bool {
	c := m.c
	progress := false
	for range c.Spec.MemoryWidth {
		msg, ok := c.Ports.Memory.RetrieveIncoming()
		if !ok {
			break
		}
		m.receive(msg)
		progress = true
	}
	for range c.Spec.MemoryWidth {
		if !c.Ports.Memory.CanSend() {
			break
		}
		if c.State.PreferUpdate && m.issueUpdate() {
			c.State.PreferUpdate = false
		} else if m.issueWalk() {
			c.State.PreferUpdate = true
		} else if m.issueUpdate() {
			c.State.PreferUpdate = false
		} else {
			break
		}
		progress = true
	}
	return progress
}

func (m *memoryMiddleware) issueWalk() bool {
	c := m.c
	for n := range c.State.Walks {
		i := (c.State.NextMemoryWalk + n) % len(c.State.Walks)
		w := &c.State.Walks[i]
		if w.Stage != readReady {
			continue
		}
		addr := c.Spec.pteAddress(w.Table, w.VPN, w.Level)
		w.markWaiters(c, tasks.MilestoneKindHardwareResource, "PTE issue")
		w.SkipFill = false
		w.MemoryID, w.MemoryTask = m.send(w.PID, addr, nil, w.Waiters[0].Task)
		w.Stage = readPending
		c.State.NextMemoryWalk = (i + 1) % len(c.State.Walks)
		return true
	}
	return false
}

func (m *memoryMiddleware) issueUpdate() bool {
	c := m.c
	u := &c.State.Update
	if u.ID == 0 || u.MemoryID != 0 {
		return false
	}
	r := u.Request
	addr := c.Spec.pteAddress(u.Table, r.VAddr>>c.Spec.Log2PageSize, u.Level)
	var data []byte
	switch u.Stage {
	case probe:
	case zeroRoot, zeroChild:
		return m.issueZero(u)
	case linkChild:
		data = binary.LittleEndian.AppendUint64(nil, u.Nodes[u.Child]|validBit)
	case writeLeaf:
		var word uint64
		if r.Command != pt.Unmap {
			word = c.Spec.encodeLeaf(r.Mapping)
		}
		data = binary.LittleEndian.AppendUint64(nil, word)
	default:
		return false
	}
	u.mark(c.TaskTracker(), tasks.MilestoneKindHardwareResource, "PTE issue")
	u.MemoryID, u.MemoryTask = m.send(r.PID, addr, data, u.Task)
	return true
}

func (m *memoryMiddleware) issueZero(u *update) bool {
	if len(u.ZeroWrites) == maxZeroWrites || u.ZeroOffset == nodeBytes {
		return false
	}
	node := u.Request.Root
	if u.Stage == zeroChild {
		node = u.Nodes[u.Child]
	}
	if u.ZeroOffset == 0 {
		u.mark(m.c.TaskTracker(), tasks.MilestoneKindHardwareResource, "PTE issue")
	}
	addr := node + u.ZeroOffset
	width := m.c.Spec.zeroWidth(addr)
	id, task := m.send(u.Request.PID, addr, make([]byte, width), u.Task)
	if u.ZeroWrites == nil {
		u.ZeroWrites = make(map[uint64]uint64, maxZeroWrites)
	}
	u.ZeroWrites[id] = task
	u.ZeroOffset += width
	return true
}

// A zeroing write stays within one aligned transfer and one memory route.
func (s Spec) zeroWidth(addr uint64) uint64 {
	width := s.MaxZeroWriteSize
	if width == 0 {
		width = defaultMaxZeroWriteSize
	}
	n := width - addr%width
	m := s.Memory
	if len(m.Destinations) > 1 {
		n = min(n, m.InterleavingSize-addr%m.InterleavingSize)
	}
	if m.RangeHigh != 0 {
		if addr < m.RangeLow {
			n = min(n, m.RangeLow-addr)
		} else if addr < m.RangeHigh {
			n = min(n, m.RangeHigh-addr)
		}
	}
	return n
}

func (m *memoryMiddleware) send(pid vm.PID, addr uint64, data []byte, parent uint64) (uint64, uint64) {
	c := m.c
	p := c.Ports.Memory
	msg := messaging.Msg{ID: c.NewID(), Src: p.AsRemote(), Dst: c.Spec.Memory.Find(addr), TrafficBytes: 12,
		Payload: memprotocol.ReadReq{PID: pid, Address: addr, AccessByteSize: pteBytes}}
	if data != nil {
		msg.Payload = memprotocol.WriteReq{PID: pid, Address: addr, Data: data}
		msg.TrafficBytes += len(data)
	}
	task := c.TaskTracker().StartReqOut(c, msg, parent)
	p.Send(msg)
	return msg.ID, task
}

func (m *memoryMiddleware) receive(msg messaging.Msg) {
	c := m.c
	u := &c.State.Update
	if msg.RspTo == 0 {
		panic("pagetablemmu: expected a memory response")
	}
	if m.receiveUpdate(u, msg) {
		return
	}
	for i := range c.State.Walks {
		w := &c.State.Walks[i]
		if w.Stage == readPending && w.MemoryID == msg.RspTo {
			w.markWaiters(c, tasks.MilestoneKindData, c.Ports.Memory.Name())
			c.TaskTracker().EndReqOut(w.MemoryTask)
			w.MemoryID = 0
			m.walkRead(w, readWord(msg))
			return
		}
	}
	panic("pagetablemmu: unmatched memory response")
}

func (m *memoryMiddleware) receiveUpdate(u *update, msg messaging.Msg) bool {
	task, zero := u.ZeroWrites[msg.RspTo]
	if zero {
		delete(u.ZeroWrites, msg.RspTo)
	} else if msg.RspTo == u.MemoryID {
		task, u.MemoryID = u.MemoryTask, 0
	} else {
		return false
	}
	m.c.TaskTracker().EndReqOut(task)
	if u.Stage == probe {
		u.mark(m.c.TaskTracker(), tasks.MilestoneKindData, m.c.Ports.Memory.Name())
		m.updateProbe(u, readWord(msg))
	} else {
		if _, ok := msg.Payload.(memprotocol.WriteDoneRsp); !ok {
			panic("pagetablemmu: expected write ACK")
		}
		m.written(u)
	}
	return true
}

func readWord(msg messaging.Msg) uint64 {
	data := msg.Payload.(memprotocol.DataReadyRsp).Data
	if len(data) != pteBytes {
		panic("pagetablemmu: expected one 8-byte PTE")
	}
	return binary.LittleEndian.Uint64(data)
}

func (m *memoryMiddleware) child(word uint64, pid vm.PID) uint64 {
	addr := word &^ uint64(validBit)
	owner, ok := m.c.State.Owners[addr]
	if !ok || owner != pid || addr%nodeBytes != 0 {
		panic("pagetablemmu: malformed or unowned child PTE")
	}
	return addr
}

func (m *memoryMiddleware) walkRead(w *walk, word uint64) {
	spec := m.c.Spec
	w.Stage = walkDone
	if word&validBit == 0 {
		return
	}
	if w.Level == len(spec.Banks) {
		if word&leafBit == 0 {
			panic("pagetablemmu: expected a leaf PTE")
		}
		w.Page = spec.decodeLeaf(word, w.PID, w.VPN)
		return
	}
	w.Table = m.child(word, w.PID)
	w.Stage = readReady
	if !w.SkipFill && len(m.c.State.Banks[w.Level]) > 0 {
		w.Stage = fillReady
	}
	w.Level++
}

func (m *memoryMiddleware) updateProbe(u *update, word uint64) {
	c := m.c
	valid := word&validBit != 0
	if u.Level == len(c.Spec.Banks) {
		switch {
		case valid && word&leafBit == 0:
			panic("pagetablemmu: expected leaf PTE")
		case valid && u.Request.Command == pt.Map:
			u.reject(fmt.Errorf("page already mapped"))
		case !valid && u.Request.Command != pt.Map:
			u.reject(fmt.Errorf("page is unmapped"))
		default:
			u.Stage = writeLeaf
		}
		return
	}
	if valid {
		u.Table = m.child(word, u.Request.PID)
		u.Level++
		return
	}
	if u.Request.Command != pt.Map {
		u.reject(fmt.Errorf("page is unmapped"))
		return
	}
	nodes, ok := c.State.reserve(c.Spec, u.Request.PID, len(c.Spec.Banks)-u.Level)
	if !ok {
		u.reject(fmt.Errorf("page-table node pool exhausted"))
		return
	}
	u.Nodes, u.Stage = nodes, zeroChild
}

func (m *memoryMiddleware) written(u *update) {
	switch u.Stage {
	case zeroRoot, zeroChild:
		if u.ZeroOffset != nodeBytes || len(u.ZeroWrites) != 0 {
			return
		}
		u.ZeroOffset = 0
		u.mark(m.c.TaskTracker(), tasks.MilestoneKindDependency, "node zeroing")
		if u.Stage == zeroRoot {
			m.c.State.Roots[u.Request.PID] = u.Request.Root
			u.Stage = ack
		} else {
			u.Stage = linkChild
		}
	case linkChild:
		u.mark(m.c.TaskTracker(), tasks.MilestoneKindSubTask, "publish child")
		u.Level++
		u.Table = u.Nodes[u.Child]
		u.Child++
		u.Stage = zeroChild
		if u.Child == len(u.Nodes) {
			u.Stage = writeLeaf
		}
	case writeLeaf:
		u.mark(m.c.TaskTracker(), tasks.MilestoneKindSubTask, "write leaf")
		u.Stage = maintain
	default:
		panic("pagetablemmu: write ACK in invalid phase")
	}
}
