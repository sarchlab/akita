package timing

import (
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
)

// serialEngineCheckpoint is the serialized form of the engine: its current time
// and the primary and secondary event queues, each encoded through the event
// codec in pop order. Event types must be registered with RegisterEvent.
type serialEngineCheckpoint struct {
	Time      VTimeInPicoSec  `json:"time"`
	Primary   json.RawMessage `json:"primary"`
	Secondary json.RawMessage `json:"secondary"`
}

// SaveCheckpoint writes the engine's current time and queued events.
// Execution and all engine mutations must be stopped until it returns.
func (e *SerialEngine) SaveCheckpoint(w io.Writer) error {
	if err := e.supervisor.Err(); err != nil {
		return err
	}
	primary, err := eventCodec.EncodeSlice(e.queue.snapshot())
	if err != nil {
		return err
	}
	secondary, err := eventCodec.EncodeSlice(e.secondaryQueue.snapshot())
	if err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(serialEngineCheckpoint{
		Time:      e.time,
		Primary:   primary,
		Secondary: secondary,
	})
}

// LoadCheckpoint restores the engine's time and event queues into a freshly
// rebuilt engine, whose queues must already be empty. Events are restored in
// pop order so the (time, sequence) ordering is reproduced, and each event's
// handler must already be registered.
func (e *SerialEngine) LoadCheckpoint(r io.Reader) (err error) {
	defer func() {
		if p := recover(); p != nil {
			e.supervisor.Fail("restore engine", p)
			err = e.supervisor.Err()
		}
		if err != nil {
			e.supervisor.Fail("restore engine", err)
			err = e.supervisor.Err()
		}
	}()
	if !e.supervisor.Fresh() {
		return fmt.Errorf("timing: restore requires a fresh engine")
	}

	if e.restored {
		return fmt.Errorf("timing: engine has already attempted a restore")
	}
	e.restored = true
	if e.queue.Len() != 0 || e.secondaryQueue.Len() != 0 {
		return fmt.Errorf(
			"timing: cannot load a checkpoint into a non-empty serial engine queue")
	}

	var dto serialEngineCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}

	primary, err := e.decodeEvents(dto.Primary)
	if err != nil {
		return err
	}
	secondary, err := e.decodeEvents(dto.Secondary)
	if err != nil {
		return err
	}

	for _, evt := range primary {
		if evt.IsSecondary() || evt.Time() < dto.Time {
			return fmt.Errorf("timing: invalid primary checkpoint event")
		}
	}
	for _, evt := range secondary {
		if !evt.IsSecondary() || evt.Time() < dto.Time {
			return fmt.Errorf("timing: invalid secondary checkpoint event")
		}
	}
	e.queue.restore(primary)
	e.secondaryQueue.restore(secondary)
	atomic.StoreUint64((*uint64)(&e.time), uint64(dto.Time))

	return nil
}

// decodeEvents decodes a queue's events through the event codec and validates
// that each restored event references a handler that exists in the rebuilt
// engine — the topology is rebuilt by setup, so a dangling handler ID means the
// checkpoint and the rebuilt simulation disagree.
func (e *SerialEngine) decodeEvents(data json.RawMessage) ([]Event, error) {
	events, err := eventCodec.DecodeSlice(data)
	if err != nil {
		return nil, err
	}

	for _, evt := range events {
		if _, ok := e.registry[evt.HandlerID()]; !ok {
			return nil, fmt.Errorf(
				"timing: restored event references unknown handler %q",
				evt.HandlerID())
		}
	}

	return events, nil
}
