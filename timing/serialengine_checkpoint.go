package timing

import (
	"encoding/json"
	"fmt"
	"io"
)

// serialEngineCheckpoint is the serialized form of the engine: its current time
// and the primary and secondary event queues, each as typed payloads in pop
// order. Event types must be registered with RegisterEvent.
type serialEngineCheckpoint struct {
	Time      VTimeInPicoSec `json:"time"`
	Primary   []TypedPayload `json:"primary"`
	Secondary []TypedPayload `json:"secondary"`
}

// SaveCheckpoint writes the engine's current time and queued events.
func (e *SerialEngine) SaveCheckpoint(w io.Writer) error {
	primary, err := encodeEvents(e.queue.snapshot())
	if err != nil {
		return err
	}
	secondary, err := encodeEvents(e.secondaryQueue.snapshot())
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
func (e *SerialEngine) LoadCheckpoint(r io.Reader) error {
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

	e.queue.restore(primary)
	e.secondaryQueue.restore(secondary)
	e.time = dto.Time

	return nil
}

// NextEventTimeForHandler returns the earliest time of a scheduled event for the
// given handler, and whether any is scheduled. After a checkpoint is restored,
// the queue is the authoritative record of pending work, so a component uses
// this to reconcile its scheduler/wakeup guard with the restored queue.
func (e *SerialEngine) NextEventTimeForHandler(
	handlerID string,
) (VTimeInPicoSec, bool) {
	primary, hasPrimary := e.queue.earliestForHandler(handlerID)
	secondary, hasSecondary := e.secondaryQueue.earliestForHandler(handlerID)

	switch {
	case hasPrimary && hasSecondary:
		if secondary < primary {
			return secondary, true
		}
		return primary, true
	case hasPrimary:
		return primary, true
	case hasSecondary:
		return secondary, true
	default:
		return 0, false
	}
}

func encodeEvents(events []Event) ([]TypedPayload, error) {
	out := make([]TypedPayload, len(events))
	for i, evt := range events {
		tp, err := EncodeEvent(evt)
		if err != nil {
			return nil, err
		}
		out[i] = tp
	}

	return out, nil
}

func (e *SerialEngine) decodeEvents(payloads []TypedPayload) ([]Event, error) {
	out := make([]Event, len(payloads))
	for i, tp := range payloads {
		evt, err := DecodeEvent(tp)
		if err != nil {
			return nil, err
		}
		if _, ok := e.registry[evt.HandlerID()]; !ok {
			return nil, fmt.Errorf(
				"timing: restored event references unknown handler %q",
				evt.HandlerID())
		}
		out[i] = evt
	}

	return out, nil
}
