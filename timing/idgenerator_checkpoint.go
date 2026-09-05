package timing

import (
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
)

// idGeneratorCheckpoint is the serialized form of the ID generator: its kind and
// next counter value.
type idGeneratorCheckpoint struct {
	Kind     string                       `json:"kind"`
	NextID   uint64                       `json:"next_id"`
	Tracking map[string]map[uint64]uint64 `json:"tracking,omitempty"`
}

// SaveCheckpoint writes the sequential ID generator's kind and next counter.
func (g *sequentialIDGenerator) SaveCheckpoint(w io.Writer) error {
	g.trackingMu.Lock()
	defer g.trackingMu.Unlock()
	dto := idGeneratorCheckpoint{
		Kind:     "sequential",
		Tracking: g.tracking,
		NextID:   atomic.LoadUint64(&g.nextID),
	}
	return json.NewEncoder(w).Encode(dto)
}

// LoadCheckpoint restores the sequential ID generator's counter.
func (g *sequentialIDGenerator) LoadCheckpoint(r io.Reader) error {
	var dto idGeneratorCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}
	if dto.Kind != "sequential" {
		return fmt.Errorf(
			"timing: ID generator kind mismatch: checkpoint %q, rebuilt sequential",
			dto.Kind)
	}
	for _, tasks := range dto.Tracking {
		for msg, id := range tasks {
			if msg == 0 || id == 0 || id > dto.NextID {
				return fmt.Errorf("timing: invalid tracked ID")
			}
		}
	}
	g.trackingMu.Lock()
	defer g.trackingMu.Unlock()
	g.tracking = dto.Tracking
	atomic.StoreUint64(&g.nextID, dto.NextID)
	return nil
}
