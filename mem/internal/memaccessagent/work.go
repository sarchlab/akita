package memaccessagent

// Accesses is the input data for the agent's initial random access stream.
// Reads verify values written by this stream, so a nonempty read stream needs
// at least one write. AddressRange is a size, relative to AddressOffset.
type Accesses struct {
	ReadCount, WriteCount       int
	AddressRange, AddressOffset uint64
	Seed                        int64
}

// Start replaces legacy initialization defaults and schedules the first tick.
// Call once after Initialize and before execution. An already started or
// in-flight workload cannot be replaced. All active data lives in checkpointed
// State; the hardware Spec remains unchanged.
func Start(a *Comp, work Accesses) {
	if a.Middlewares.Agent == nil {
		panic("memaccessagent: Start requires Initialize")
	}
	if a.State.Started || len(a.State.PendingReadReq) != 0 || len(a.State.PendingWriteReq) != 0 {
		panic("memaccessagent: workload already started")
	}
	if work.ReadCount < 0 || work.WriteCount < 0 || (work.ReadCount > 0 && work.WriteCount == 0) {
		panic("memaccessagent: nonnegative counts required; reads require writes")
	}
	if work.AddressRange < 4 || work.AddressOffset%4 != 0 || work.AddressOffset+work.AddressRange < work.AddressOffset {
		panic("memaccessagent: invalid aligned address range")
	}
	a.State = State{
		MaxAddress: work.AddressRange, AddressOffset: work.AddressOffset, Started: true,
		ReadLeft: work.ReadCount, WriteLeft: work.WriteCount,
		KnownMemValue: make(map[uint64][]uint32), PendingReadReq: make(map[uint64]pendingRequest),
		PendingWriteReq: make(map[uint64]pendingRequest), RNG: newRandState(work.Seed),
	}
	a.TickLater()
}
