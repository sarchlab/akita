package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// eventDrivenCheckpoint is the serialized form of an event-driven component: a
// spec hash for compatibility checking plus the mutable State. Ports and
// resources are rebuilt by setup, not serialized. The wakeup guard
// (pendingWakeup) is not serialized: it is derived from the restored event queue
// in AfterCheckpointLoad, since the queue (not the rebuilt guard, which starts
// empty) is the authoritative record of whether a wakeup is pending.
type eventDrivenCheckpoint struct {
	SpecHash string          `json:"spec_hash"`
	State    json.RawMessage `json:"state"`
}

// SaveCheckpoint writes the component's spec hash and State as JSON. It
// implements the structural Checkpointable contract without importing the
// simulation package.
func (c *EventDrivenComponent[S, T, R]) SaveCheckpoint(w io.Writer) error {
	state, err := json.Marshal(c.State)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	return json.NewEncoder(w).Encode(eventDrivenCheckpoint{
		SpecHash: c.specHash(),
		State:    state,
	})
}

// LoadCheckpoint restores State after verifying that the saved spec hash matches
// the rebuilt component's.
func (c *EventDrivenComponent[S, T, R]) LoadCheckpoint(r io.Reader) error {
	var dto eventDrivenCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode event-driven checkpoint: %w", err)
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

	return nil
}

// AfterCheckpointLoad reconciles the wakeup guard with the restored event queue.
// It runs after every entity — including the engine — has loaded its raw state,
// so the queue is fully restored and authoritative: a wakeup is pending iff the
// engine holds a timer event for this handler, at that event's time. This keeps
// a post-restore request for the same (or a later) wakeup from scheduling a
// duplicate.
func (c *EventDrivenComponent[S, T, R]) AfterCheckpointLoad() error {
	querier, ok := c.engine.(pendingEventQuerier)
	if !ok {
		return nil
	}

	c.Lock()
	defer c.Unlock()

	if when, has := querier.NextEventTimeForHandler(c.Name()); has {
		c.pendingWakeup = when
	} else {
		c.pendingWakeup = math.MaxUint64
	}

	return nil
}

// specHash is a deterministic fingerprint of the component's immutable Spec, used
// to reject loading a checkpoint into a component built with a different config.
func (c *EventDrivenComponent[S, T, R]) specHash() string {
	data, err := json.Marshal(c.spec)
	if err != nil {
		panic(fmt.Sprintf("modeling: cannot hash spec: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
