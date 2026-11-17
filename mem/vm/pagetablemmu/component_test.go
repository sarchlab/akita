package pagetablemmu

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	at "github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	pf "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestMemoryWalkAndPrefixHits(t *testing.T) {
	r := newRig(t, baseSpec(), nil, false)
	r.root(r.mmu)
	r.install(r.mmu, 0x4000, 0x80000, 3)
	r.traffic.Items = nil
	start := r.s.Engine().CurrentTime()
	first := r.translate(0x4004, 3)
	second := r.translate(0x4ffc, 2)
	r.run()
	elapsed := r.s.Engine().CurrentTime() - start
	for _, id := range []uint64{first, second} {
		msg := r.recv(r.request)
		require.Equal(t, id, msg.RspTo)
		p := msg.Payload.(at.Rsp).Page
		require.Equal(t, vm.Page{Valid: true, PID: 7, VAddr: 0x4000, PAddr: 0x80000,
			DeviceID: 3, PageSize: 4096, IsPinned: true, Unified: true}, p)
	}
	cold := r.reads(r.mmu.Ports.Memory)
	require.Len(t, cold, 4)
	require.Equal(t, r.mmu.Spec.PoolBase, cold[0])
	for i := range 3 {
		pointer := binary.LittleEndian.Uint64(r.storage.Read(cold[i], 8))
		require.Equal(t, pointer&^uint64(1), cold[i+1]&^uint64(4095))
	}
	r.traffic.Items = nil
	start = r.s.Engine().CurrentTime()
	r.translate(0x4000, 3)
	r.run()
	warm := r.s.Engine().CurrentTime() - start
	require.Equal(t, cold[3:], r.reads(r.mmu.Ports.Memory))
	require.Less(t, warm, elapsed)
	t.Logf("cold pair: four dependent PTE reads in %d ps. Warm: one read in %d ps", elapsed, warm)
}

func TestFaultDecisionsAndBackpressure(t *testing.T) {
	local := baseSpec()
	local.NumWalkEntries, local.MaxWaitersPerEntry = 1, 3
	ioSpec := baseSpec()
	ioSpec.PoolBase = 0x38000
	ioSpec.ConsultRemote = true
	r := newRig(t, local, &ioSpec, false)
	r.root(r.mmu)
	r.root(r.io)
	r.install(r.io, 0x7000, 0x90000, 1)
	r.traffic.Items = nil
	first := r.translate(0x7123, 2)
	second := r.translate(0x7abc, 2)
	third := r.translate(0x7040, 3)
	fourth := r.translate(0x7050, 2)
	r.run()
	require.Equal(t, 1, r.mmu.Ports.Top.IncomingBuf().Size())
	require.Zero(t, r.request.IncomingBuf().Size())
	require.Len(t, r.reads(r.io.Ports.Memory), 4)
	decisions := map[uint64]messaging.Msg{}
	for range 2 {
		msg := r.recv(r.policy)
		req := msg.Payload.(pf.Req)
		require.True(t, req.HasMapping)
		require.Equal(t, uint64(0x90000), req.Mapping.PAddr)
		decisions[req.DeviceID] = msg
	}
	require.Equal(t, uint64(0x7123), decisions[2].Payload.(pf.Req).VAddr)
	require.Equal(t, uint64(0x7040), decisions[3].Payload.(pf.Req).VAddr)
	r.send(r.policy, decisions[3].Src, pf.Rsp{Result: pf.Blocked}, decisions[3].ID)
	r.run()
	msg := r.recv(r.request)
	require.Equal(t, third, msg.RspTo)
	require.True(t, msg.Payload.(at.Rsp).Fault)
	require.Equal(t, uint64(0x7000), msg.Payload.(at.Rsp).Page.VAddr)
	require.Zero(t, r.mmu.Ports.Top.IncomingBuf().Size())
	require.Zero(t, r.policy.IncomingBuf().Size())
	resolved := decisions[2].Payload.(pf.Req).Mapping
	r.send(r.policy, decisions[2].Src, pf.Rsp{Result: pf.Resolved, Mapping: resolved}, decisions[2].ID)
	r.run()
	for _, id := range []uint64{first, second, fourth} {
		msg = r.recv(r.request)
		require.Equal(t, id, msg.RspTo)
		require.Equal(t, at.Rsp{Page: resolved}, msg.Payload)
	}
	r.translate(0x7051, 3)
	r.run()
	require.Equal(t, uint64(0x7051), r.recv(r.policy).Payload.(pf.Req).VAddr)
	t.Log("four requests shared one fallback walk and two device decisions. A fresh request reached policy after NACK")
}

func TestTLBDrainsBlockedAndRetranslates(t *testing.T) {
	r := newRig(t, baseSpec(), nil, true)
	r.root(r.mmu)
	id := r.translate(0xb000, 2)
	r.run()
	f := r.recv(r.policy)
	require.False(t, f.Payload.(pf.Req).HasMapping)
	r.send(r.policy, f.Src, pf.Rsp{Result: pf.Blocked}, f.ID)
	r.run()
	msg := r.recv(r.request)
	require.Equal(t, id, msg.RspTo)
	require.True(t, msg.Payload.(at.Rsp).Fault)
	r.install(r.mmu, 0xb000, 0x81000, 2)
	id = r.translate(0xb000, 2)
	r.run()
	msg = r.recv(r.request)
	require.Equal(t, id, msg.RspTo)
	require.False(t, msg.Payload.(at.Rsp).Fault)
	require.Equal(t, uint64(0x81000), msg.Payload.(at.Rsp).Page.PAddr)
}

func TestCheckpointDuringNodePublication(t *testing.T) {
	spec := baseSpec()
	spec.Banks = append(spec.Banks, BankSpec{NumSets: 1, NumWays: 1}) // 48 VA bits with fixed top index.
	path := filepath.Join(t.TempDir(), "mid-map.tar.gz")
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	cmd := pt.Req{Command: pt.Map, PID: 7, VAddr: 0x7000, Mapping: vm.Page{PAddr: 0x80000}}
	id := r.send(r.ctrl, "Local.Control", cmd, 0)
	engine := r.s.Engine().(*timing.SerialEngine)
	require.NoError(t, engine.RunUntil(engine.CurrentTime()+50000))
	require.Equal(t, zeroChild, r.mmu.State.Update.Stage)
	require.Zero(t, r.ctrl.IncomingBuf().Size())
	require.NoError(t, r.s.SaveCheckpoint(path))
	r.traffic.Items = nil
	r.run()
	acknowledgment := r.recv(r.ctrl)
	require.Equal(t, id, acknowledgment.RspTo)
	restored := newRig(t, spec, nil, false)
	require.NoError(t, restored.s.LoadCheckpoint(path))
	restored.run()
	require.Equal(t, acknowledgment, restored.recv(restored.ctrl))
	require.Equal(t, r.traffic.Items, restored.traffic.Items)
	require.Equal(t, r.mmu.State, restored.mmu.State)
	require.Equal(t, r.storage.Read(spec.PoolBase, 5*nodeBytes), restored.storage.Read(spec.PoolBase, 5*nodeBytes))
	restored.traffic.Items = nil
	restored.translate(0x7000, 0)
	restored.run()
	require.Len(t, restored.reads(restored.mmu.Ports.Memory), 5)
	t.Logf("mid-child restore reproduced %d messages with identical times and IDs", len(r.traffic.Items))
}

func TestRangeProtectionDrainsThenPublishes(t *testing.T) {
	spec := baseSpec()
	spec.ConsultRemote = true
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	r.install(r.mmu, 0x4000, 0x80000, 1)
	r.install(r.mmu, 0x5000, 0x81000, 2)
	original := r.translate(0x4008, 2)
	r.run()
	f := r.recv(r.policy)
	cmdID := r.send(r.ctrl, "Local.Control", pt.Req{Command: pt.Remap, PID: 7, VAddr: 0x4000,
		Mapping: vm.Page{PAddr: 0x91000, DeviceID: 2}}, 0)
	later := r.translate(0x400c, 2)
	// A different input port lets independent work pass the stalled Top head.
	independent := r.send(r.request, "Local.FaultIn", pf.Req{PID: 7, VAddr: 0x5004, DeviceID: 2}, 0)
	r.run()
	require.Equal(t, drain, r.mmu.State.Update.Stage)
	require.Zero(t, r.ctrl.IncomingBuf().Size())
	msg := r.recv(r.request)
	require.Equal(t, independent, msg.RspTo)
	require.Equal(t, uint64(0x81000), msg.Payload.(pf.Rsp).Mapping.PAddr)
	r.send(r.policy, f.Src, pf.Rsp{Result: pf.Blocked}, f.ID)
	r.run()
	msg = r.recv(r.request)
	require.Equal(t, original, msg.RspTo)
	require.True(t, msg.Payload.(at.Rsp).Fault)
	msg = r.recv(r.request)
	require.Equal(t, later, msg.RspTo)
	require.Equal(t, uint64(0x91000), msg.Payload.(at.Rsp).Page.PAddr)
	require.Equal(t, cmdID, r.recv(r.ctrl).RspTo)
}

func TestPoolExhaustionRetentionAndTeardown(t *testing.T) {
	spec := baseSpec()
	spec.PoolNodes = 4
	r := newRig(t, spec, nil, false)
	r.storage.Write(spec.PoolBase, []byte("old root"))
	r.root(r.mmu)
	require.Equal(t, make([]byte, 8), r.storage.Read(spec.PoolBase, 8))
	r.install(r.mmu, 0x4000, 0x80000, 1)
	outOfRange := uint64(1) << 39
	noRoom := pt.Req{Command: pt.Map, PID: 7, VAddr: outOfRange, Mapping: vm.Page{PAddr: 0x81000}}
	require.Contains(t, r.cmd(r.mmu, noRoom), "exhausted")
	require.Equal(t, make([]byte, 8), r.storage.Read(spec.PoolBase+8, 8))
	require.Empty(t, r.cmd(r.mmu, pt.Req{Command: pt.Unmap, PID: 7, VAddr: 0x4000}))
	require.Len(t, r.mmu.State.Owners, 4)
	r.install(r.mmu, 0x4000, 0x83000, 1)
	require.Contains(t, r.cmd(r.mmu, pt.Req{Command: pt.Map, PID: 7, VAddr: 0x4000, Mapping: vm.Page{}}), "already mapped")
	require.Empty(t, r.cmd(r.mmu, pt.Req{Command: pt.Teardown, PID: 7}))
	require.Empty(t, r.mmu.State.Owners)
	require.Empty(t, r.cmd(r.mmu, pt.Req{Command: pt.RegisterRoot, PID: 8, Root: spec.PoolBase}))
	noRoom.PID = 8
	require.Empty(t, r.cmd(r.mmu, noRoom))
	r.send(r.request, "Local.Top", at.Req{PID: 8, VAddr: 0x4000}, 0)
	r.run()
	require.False(t, r.recv(r.policy).Payload.(pf.Req).HasMapping)
}

func TestCuckooEvictionUsesAuthoritativeMapping(t *testing.T) {
	local := baseSpec()
	local.FilterBuckets, local.FilterMaxKicks = 1, 1
	io := baseSpec()
	io.PoolBase = 0x38000
	io.ConsultRemote = true
	r := newRig(t, local, &io, false)
	r.root(r.mmu)
	r.root(r.io)
	for _, c := range []*Comp{r.mmu, r.io} {
		r.install(c, 0x4000, 0x80000, 2)
		r.install(c, 0x5000, 0x81000, 2)
	}
	require.False(t, r.mmu.State.filterContains(7, 4))
	require.True(t, r.mmu.State.filterContains(7, 5))
	r.traffic.Items = nil
	r.translate(0x4042, 2)
	r.run()
	require.Empty(t, r.reads(r.mmu.Ports.Memory))
	require.Len(t, r.reads(r.io.Ports.Memory), 4)
	require.Zero(t, r.policy.IncomingBuf().Size())
	require.Equal(t, uint64(0x80000), r.recv(r.request).Payload.(at.Rsp).Page.PAddr)
	require.True(t, r.mmu.State.filterContains(7, 5))
	// A positive fingerprint for an unmapped VPN goes through both walkers.
	var alias uint64
	for alias = 6; alias < 1<<22; alias++ {
		if r.mmu.State.filterContains(7, alias) {
			break
		}
	}
	require.Less(t, alias, uint64(1<<22))
	r.traffic.Items = nil
	r.translate(alias<<12, 2)
	r.run()
	require.NotEmpty(t, r.reads(r.mmu.Ports.Memory))
	require.NotEmpty(t, r.reads(r.io.Ports.Memory))
	req := r.recv(r.policy).Payload.(pf.Req)
	require.Equal(t, alias<<12, req.VAddr)
	require.False(t, req.HasMapping)
}

func TestSplitCacheLookupAndFillBandwidth(t *testing.T) {
	for _, tc := range []struct {
		lookup, fill            int
		firstEdges, secondEdges int
	}{
		{1, 4, 4, 4}, {4, 1, 1, 4}, {4, 4, 1, 1},
	} {
		spec := baseSpec()
		spec.LookupWidth, spec.FillWidth = tc.lookup, tc.fill
		spec.AdmissionWidth, spec.MemoryWidth = 4, 4
		r := newRig(t, spec, nil, false)
		r.root(r.mmu)
		for i := range uint64(4) {
			r.install(r.mmu, i<<12, 0x80000+i*4096, 2)
		}
		r.traffic.Items = nil
		for i := range uint64(4) {
			r.translate(i<<12, 2)
		}
		r.run()
		rootAddr := spec.PoolBase
		childAddr := binary.LittleEndian.Uint64(r.storage.Read(rootAddr, 8)) &^ uint64(1)
		edges := map[uint64]map[timing.VTimeInPicoSec]bool{rootAddr: {}, childAddr: {}}
		for _, item := range r.traffic.Items {
			if req, ok := item.Msg.Payload.(memprotocol.ReadReq); ok && edges[req.Address] != nil {
				edges[req.Address][item.Time] = true
			}
		}
		require.Len(t, edges[rootAddr], tc.firstEdges)
		require.Len(t, edges[childAddr], tc.secondEdges)
	}
}

func TestGeometryAndBindingValidation(t *testing.T) {
	s := baseSpec()
	s.Log2PageSize = 16
	require.NoError(t, s.validate())
	s.Banks = append(s.Banks, BankSpec{NumSets: 1, NumWays: 1})
	s.VABits = 61
	require.NoError(t, s.validate())
	s.VABits = 62
	require.Error(t, s.validate())
	s = baseSpec()
	s.Banks[1] = BankSpec{NumWays: 2}
	require.Error(t, s.validate())
	require.Equal(t, 8, DefaultSpec().Banks[1].NumSets)
	s = baseSpec()
	s.FilterBuckets = 7
	require.Error(t, s.validate())
	s = baseSpec()
	s.Memory.InterleavingSize = 4
	s.Memory.Destinations = append(s.Memory.Destinations, "DRAM2.Top")
	require.Error(t, s.validate())
	s = baseSpec()
	s.FaultHandler = ""
	require.Error(t, s.validate())
}

func TestCheckpointWithPendingDeviceDecision(t *testing.T) {
	spec := baseSpec()
	r := newRig(t, spec, nil, false)
	first := r.translate(0x4123, 2)
	second := r.translate(0x4abc, 2)
	r.run()
	require.Equal(t, 1, r.policy.IncomingBuf().Size())
	path := filepath.Join(t.TempDir(), "pending-fault.tar.gz")
	require.NoError(t, r.s.SaveCheckpoint(path))
	restored := newRig(t, spec, nil, false)
	require.NoError(t, restored.s.LoadCheckpoint(path))
	for _, sim := range []*rig{r, restored} {
		req := sim.recv(sim.policy)
		sim.send(sim.policy, req.Src, pf.Rsp{Result: pf.Blocked}, req.ID)
		sim.run()
		for _, id := range []uint64{first, second} {
			msg := sim.recv(sim.request)
			require.Equal(t, id, msg.RspTo)
			require.True(t, msg.Payload.(at.Rsp).Fault)
		}
		require.Equal(t, free, sim.mmu.State.Walks[0].Stage)
	}
	require.Equal(t, r.s.Engine().CurrentTime(), restored.s.Engine().CurrentTime())
}

func TestInvalidateRangeDropsOnlyOverlappingPrefixes(t *testing.T) {
	spec := baseSpec()
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	const far = uint64(1) << 39
	for _, va := range []uint64{0x1000, 0x2000, far} {
		r.install(r.mmu, va, 0x80000, 2)
	}
	for _, va := range []uint64{0x1000, far} {
		r.translate(va, 2)
		r.run()
		r.recv(r.request)
	}
	require.Empty(t, r.cmd(r.mmu, pt.Req{Command: pt.Invalidate, PID: 7, VAddr: 0x1000, Size: 8192}))
	for _, tc := range []struct {
		va    uint64
		reads int
	}{{far, 1}, {0x2000, 4}} {
		r.traffic.Items = nil
		r.translate(tc.va, 2)
		r.run()
		r.recv(r.request)
		require.Len(t, r.reads(r.mmu.Ports.Memory), tc.reads)
	}
}

type arbiterObserver struct {
	c                        *Comp
	WalkGrants, UpdateGrants int
}

func (o *arbiterObserver) Func(h hooking.HookCtx) {
	if h.Pos != messaging.HookPosPortMsgSend {
		return
	}
	u := &o.c.State.Update
	if u.Stage != zeroChild || u.MemoryID != 0 {
		return
	}
	for i := range o.c.State.Walks {
		if o.c.State.Walks[i].Stage != readReady {
			continue
		}
		switch h.Item.(messaging.Msg).Payload.(type) {
		case memprotocol.ReadReq:
			o.WalkGrants++
		case memprotocol.WriteReq:
			o.UpdateGrants++
		}
		return
	}
}

func TestWalkAndUpdateShareMemoryIssue(t *testing.T) {
	spec := baseSpec()
	spec.AdmissionWidth, spec.LookupWidth, spec.FillWidth = 16, 16, 16
	spec.MemoryWidth = 1
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	for i := range uint64(16) {
		r.install(r.mmu, i*4096, 0x80000+i*4096, 2)
	}
	// A disjoint map continuously needs zero-fill writes while sixteen
	// independent translations compete for the same memory issue slot.
	cmd := pt.Req{Command: pt.Map, PID: 7, VAddr: uint64(1) << 39, Mapping: vm.Page{PAddr: 0x90000}}
	r.send(r.ctrl, "Local.Control", cmd, 0)
	engine := r.s.Engine().(*timing.SerialEngine)
	require.NoError(t, engine.RunUntil(engine.CurrentTime()+40000))
	require.Equal(t, zeroChild, r.mmu.State.Update.Stage)
	observer := &arbiterObserver{c: r.mmu}
	r.mmu.Ports.Memory.AcceptHook(observer)
	start := len(r.traffic.Items)
	for i := range uint64(16) {
		r.translate(i*4096, 2)
	}
	r.run()
	require.Equal(t, 16, r.request.IncomingBuf().Size())
	require.Empty(t, r.recv(r.ctrl).Payload.(pt.Rsp).Error)
	require.Positive(t, observer.UpdateGrants, "update must win an issue slot with walks ready")
	// When the update issues a write, it must wait for the real memory ACK.
	// Walks continue during those intervals, well before map completion.
	seenWalk := false
	seenWriteAfterWalk := false
	last := timing.VTimeInPicoSec(0)
	for _, item := range r.traffic.Items[start:] {
		if item.Msg.Src != r.mmu.Ports.Memory.AsRemote() {
			continue
		}
		require.NotEqual(t, last, item.Time, "two requests exceeded the one-wide memory port")
		last = item.Time
		switch item.Msg.Payload.(type) {
		case memprotocol.ReadReq:
			seenWalk = true
		case memprotocol.WriteReq:
			seenWriteAfterWalk = seenWalk
		}
	}
	require.True(t, seenWriteAfterWalk)
}

func TestCompletedWalkWaitsForReplyPort(t *testing.T) {
	spec := baseSpec()
	spec.MaxWaitersPerEntry, spec.NumWalkEntries = 2, 1
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	r.install(r.mmu, 0x1000, 0x80000, 2)
	r.install(r.mmu, 0x2000, 0x81000, 2)
	// Fill the requester's receive buffer and the MMU's outgoing buffer.
	ids := make([]uint64, 0, 34)
	for range 2 {
		for range 16 {
			ids = append(ids, r.translate(0x1001, 2))
		}
		r.run()
	}
	require.Equal(t, 16, r.request.IncomingBuf().Size())
	require.Equal(t, 16, r.mmu.Ports.Top.OutgoingBuf().Size())
	ids = append(ids, r.translate(0x1001, 2), r.translate(0x2001, 2))
	r.run()
	require.Equal(t, walkDone, r.mmu.State.Walks[0].Stage)
	require.Len(t, r.mmu.State.Walks[0].Waiters, 1)
	require.Equal(t, 1, r.mmu.Ports.Top.IncomingBuf().Size(), "distinct VPN must wait for the occupied entry")
	var got []uint64
	for len(got) < len(ids) {
		n := r.request.IncomingBuf().Size()
		require.Positive(t, n)
		r.onNextEdge(func() {
			for range n {
				got = append(got, r.recv(r.request).RspTo)
			}
		})
		r.run()
	}
	require.Equal(t, ids, got)
	require.Equal(t, free, r.mmu.State.Walks[0].Stage)
}
