package writethrough

import "slices"

func (m *ctrlMiddleware) advanceMaintenance() bool {
	c := m.comp
	s := &c.State
	op := &s.Maintenance
	if !op.AccessesDrained {
		for i := range s.Transactions {
			t := &s.Transactions[i]
			if !t.Removed && !slices.Contains(op.Deferred, i) && op.Matches(t.PID(), t.Address(), c.Spec.Log2BlockSize) {
				return false
			}
		}
		op.AccessesDrained = true
		return true
	}
	if len(op.Lines) > 0 || op.LookupsInFlight > 0 || !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(c, op.Command, op.Src, op.ID, true, ""))
	c.TaskTracker().EndReqIn(op.RecvTaskID)
	op.Finish()
	return true
}

func (s *intake) issueMaintenanceLookup() bool {
	next := &s.cache.comp.State
	op := &next.Maintenance
	if !next.DirBuf.CanPush() {
		return false
	}
	idx := s.allocTransaction(next, transactionState{
		MaintenanceLookup: true, LookupAddress: op.Lines[0], RecvTaskID: op.RecvTaskID,
	})
	next.DirBuf.Push(idx)
	op.Lines = op.Lines[1:]
	op.LookupsInFlight++
	return true
}
