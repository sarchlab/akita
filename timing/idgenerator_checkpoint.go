package timing

import (
	"encoding/json"
	"io"
	"sync/atomic"
)

type idGeneratorCheckpoint struct {
	NextID uint64 `json:"next_id"`
}

// SaveCheckpoint writes this simulation's ID counter. The simulation must be
// stopped so this snapshot is consistent with its other entities.
func (g *IDGenerator) SaveCheckpoint(w io.Writer) error {
	return json.NewEncoder(w).Encode(idGeneratorCheckpoint{
		NextID: atomic.LoadUint64(&g.nextID),
	})
}

// LoadCheckpoint restores the counter into a fresh, stopped target. Other
// simulations' counters are independent and are not changed.
func (g *IDGenerator) LoadCheckpoint(r io.Reader) error {
	var dto idGeneratorCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}
	atomic.StoreUint64(&g.nextID, dto.NextID)
	return nil
}
