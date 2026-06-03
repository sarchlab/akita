package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/timing"
)

// componentCheckpoint is the serialized form of a generic component: a spec hash
// for compatibility checking plus the mutable runtime (State and the tick
// scheduler guard). Resources and ports are rebuilt by setup, not serialized.
type componentCheckpoint struct {
	SpecHash         string            `json:"spec_hash"`
	State            json.RawMessage   `json:"state"`
	NextTickTime     timing.VTimeInSec `json:"next_tick_time"`
	HasScheduledTick bool              `json:"has_scheduled_tick"`
}

// SaveCheckpoint writes the component's spec hash, State, and tick-scheduler
// guard as JSON. It implements the structural checkpoint.Checkpointable contract
// without the modeling package importing checkpoint.
func (c *Component[S, T, R]) SaveCheckpoint(w io.Writer) error {
	state, err := json.Marshal(c.State)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	dto := componentCheckpoint{
		SpecHash: c.specHash(),
		State:    state,
	}
	if c.TickingComponent != nil && c.TickScheduler != nil {
		c.TickScheduler.lock.Lock()
		dto.NextTickTime = c.TickScheduler.nextTickTime
		dto.HasScheduledTick = c.TickScheduler.hasScheduledTick
		c.TickScheduler.lock.Unlock()
	}

	return json.NewEncoder(w).Encode(dto)
}

// LoadCheckpoint restores State and the tick-scheduler guard after verifying that
// the saved spec hash matches the rebuilt component's. It schedules no tick: the
// restored engine queue is authoritative.
func (c *Component[S, T, R]) LoadCheckpoint(r io.Reader) error {
	var dto componentCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode component checkpoint: %w", err)
	}

	if got := c.specHash(); got != dto.SpecHash {
		return fmt.Errorf(
			"modeling: spec hash mismatch: checkpoint %s, rebuilt %s",
			dto.SpecHash, got)
	}

	var state T
	if err := json.Unmarshal(dto.State, &state); err != nil {
		return fmt.Errorf("modeling: unmarshal state: %w", err)
	}
	c.State = state

	if c.TickingComponent != nil && c.TickScheduler != nil {
		c.TickScheduler.lock.Lock()
		c.TickScheduler.nextTickTime = dto.NextTickTime
		c.TickScheduler.hasScheduledTick = dto.HasScheduledTick
		c.TickScheduler.lock.Unlock()
	}

	return nil
}

// specHash is a deterministic fingerprint of the component's immutable Spec, used
// to reject loading a checkpoint into a component built with a different config.
func (c *Component[S, T, R]) specHash() string {
	data, err := json.Marshal(c.spec)
	if err != nil {
		panic(fmt.Sprintf("modeling: cannot hash spec: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
