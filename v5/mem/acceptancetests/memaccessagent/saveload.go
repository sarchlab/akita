package memaccessagent

import (
	"encoding/json"
	"io"

	"github.com/sarchlab/akita/v5/sim"
)

// agentSnapshot is the serializable representation of a MemAccessAgent.
type agentSnapshot struct {
	WriteLeft    int                  `json:"write_left"`
	ReadLeft     int                  `json:"read_left"`
	MaxAddress   uint64               `json:"max_address"`
	KnownMemValue map[uint64][]uint32 `json:"known_mem_value"`
}

// SaveState marshals the agent's state as JSON and writes it to w.
// Must be called at quiescence (PendingReadReq and PendingWriteReq empty).
func (a *MemAccessAgent) SaveState(w io.Writer) error {
	snap := agentSnapshot{
		WriteLeft:     a.WriteLeft,
		ReadLeft:      a.ReadLeft,
		MaxAddress:    a.MaxAddress,
		KnownMemValue: a.KnownMemValue,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// LoadState reads JSON from r and restores the agent's state.
func (a *MemAccessAgent) LoadState(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var snap agentSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	a.WriteLeft = snap.WriteLeft
	a.ReadLeft = snap.ReadLeft
	a.MaxAddress = snap.MaxAddress
	a.KnownMemValue = snap.KnownMemValue
	a.PendingReadReq = make(map[string]*sim.GenericMsg)
	a.PendingWriteReq = make(map[string]*sim.GenericMsg)

	return nil
}

// ResetTick resets the TickScheduler so that future TickLater calls can
// schedule new events.
func (a *MemAccessAgent) ResetTick() {
	a.TickScheduler.Reset()
}

// ResetAndRestartTick resets the TickScheduler and schedules a new tick.
func (a *MemAccessAgent) ResetAndRestartTick() {
	a.TickScheduler.Reset()
	a.TickLater()
}
