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
// for compatibility checking plus the mutable State. Resources and ports are
// rebuilt by setup, not serialized. The tick-scheduler guard is not serialized:
// it is derived from the restored event queue in AfterCheckpointLoad, since the
// queue (not the live guard, which can be stale) is the authoritative record of
// whether a tick is pending.
type componentCheckpoint struct {
	SpecHash string          `json:"spec_hash"`
	State    json.RawMessage `json:"state"`
}

// pendingEventQuerier is implemented by engines that can report the next
// scheduled event time for a handler. After a checkpoint is restored, the event
// queue is authoritative, so a scheduler reconciles its guard against it.
type pendingEventQuerier interface {
	NextEventTimeForHandler(handlerID string) (timing.VTimeInPicoSec, bool)
}

// SaveCheckpoint writes the component's spec hash and State as JSON. It
// implements the structural checkpoint.Checkpointable contract without the
// modeling package importing checkpoint.
func (c *Component[S, T, R]) SaveCheckpoint(w io.Writer) error {
	state, err := json.Marshal(c.State)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	return json.NewEncoder(w).Encode(componentCheckpoint{
		SpecHash: c.specHash(),
		State:    state,
	})
}

// LoadCheckpoint restores State after verifying that the saved spec hash matches
// the rebuilt component's.
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

	return nil
}

// AfterCheckpointLoad reconciles the tick-scheduler guard with the restored
// event queue. It runs after every entity — including the engine — has loaded
// its raw state, so the queue is fully restored and authoritative.
func (c *Component[S, T, R]) AfterCheckpointLoad() error {
	if c.TickingComponent != nil && c.TickScheduler != nil {
		c.TickScheduler.restoreGuardFromQueue()
	}

	return nil
}

// restoreGuardFromQueue sets the tick-scheduler guard to agree with the restored
// event queue: a tick is considered pending iff the engine holds an event for
// this handler, and nextTickTime is that event's time. This reproduces the
// guard state of an uninterrupted run, so a stimulus arriving before the pending
// tick fires does not schedule a duplicate.
func (t *TickScheduler) restoreGuardFromQueue() {
	querier, ok := t.engine.(pendingEventQuerier)
	if !ok {
		return
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	if when, has := querier.NextEventTimeForHandler(t.handlerID); has {
		t.nextTickTime = when
		t.hasScheduledTick = true
	} else {
		t.nextTickTime = 0
		t.hasScheduledTick = false
	}
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
