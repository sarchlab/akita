package writeback

import (
	"slices"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/cache/internal/maintenance"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

func (m *ctrlMiddleware) startMaintenance(msg messaging.Msg) bool {
	c := m.pipeline.comp
	c.State.Maintenance.Start(msg, c.Spec.Log2BlockSize, c.TaskTracker().StartReqIn(c, msg))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// Once older matching requests finish, the top parser can issue ordinary
// directory lookups for the specified lines. New matching requests wait in
// the bounded deferred queue; unrelated requests continue through the pipeline.
func (m *ctrlMiddleware) advanceMaintenance() bool {
	c := m.pipeline.comp
	s := &c.State
	op := &s.Maintenance
	if !op.AccessesDrained {
		for i := range s.Transactions {
			t := &s.Transactions[i]
			if t.Removed || slices.Contains(op.Deferred, i) {
				continue
			}
			if t.HasRead && op.Matches(t.ReadPID, t.ReadAddress, c.Spec.Log2BlockSize) ||
				t.HasWrite && op.Matches(t.WritePID, t.WriteAddress, c.Spec.Log2BlockSize) {
				return false
			}
		}
		op.AccessesDrained = true
		return true
	}
	if len(s.FlusherBlockToEvictRefs) > 0 {
		return issueFlushWriteback(c, op.RecvTaskID)
	}
	if len(op.Lines) > 0 || op.LookupsInFlight > 0 || !m.ctrlPort().CanSend() {
		return false
	}
	// A replacement can already have reassigned a way while its victim's
	// write is still queued in a bank or awaiting a lower-memory write ACK.
	for i := range s.Transactions {
		t := &s.Transactions[i]
		if !t.HasVictim || !op.Matches(t.VictimPID, t.VictimTag, c.Spec.Log2BlockSize) {
			continue
		}
		// An eviction is either still on its way to Bottom, or issued
		// and awaiting its response. A completed transaction can retain
		// its victim metadata until the slot is reused.
		if t.EvictionWriteReqMeta.ID == 0 || slices.Contains(s.InflightEvictionIndices, i) {
			return false
		}
	}
	m.ctrlPort().Send(makeCtrlRsp(c, op.Command, op.Src, op.ID, true, ""))
	c.TaskTracker().EndReqIn(op.RecvTaskID)
	op.Finish()
	return true
}

func (p *topParser) issueMaintenanceLookup() bool {
	s := &p.cache.comp.State
	op := &s.Maintenance
	if !s.DirStageBuf.CanPush() {
		return false
	}
	idx := s.allocTransaction(transactionState{
		MaintenanceLookup: true, LookupAddress: op.Lines[0], RecvTaskID: op.RecvTaskID,
	})
	s.DirStageBuf.Push(idx)
	op.Lines = op.Lines[1:]
	op.LookupsInFlight++
	return true
}

func (ds *directoryStage) processMaintenanceLookup(t *transactionState) {
	c := ds.cache.comp
	s := &c.State
	op := &s.Maintenance
	op.VisitLine(&s.DirectoryState, 1<<c.Spec.Log2BlockSize, t.LookupAddress, func(block *cache.BlockState) {
		if op.Command == memcontrolprotocol.CmdInvalidate {
			maintenance.Invalidate(block)
		} else if block.IsDirty {
			// A concurrent miss in the same set must not replace this way
			// while its bank read and writeback are in progress.
			block.IsLocked = true
			s.FlusherBlockToEvictRefs = append(s.FlusherBlockToEvictRefs, blockRef{SetID: block.SetID, WayID: block.WayID})
		}
	})
	ds.popDirPostBuf()
	t.Removed = true
	op.LookupsInFlight--
}
