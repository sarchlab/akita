package messaging

import (
	"encoding/json"
	"fmt"
	"io"
)

// portCheckpoint is the serialized form of a port: the buffer capacities (a
// shape check) plus the in-flight messages in each buffer, encoded as typed
// payloads so their concrete types survive the round trip. Hooks and the owning
// component/connection are rebuilt by setup and not serialized.
type portCheckpoint struct {
	IncomingCapacity int            `json:"incoming_capacity"`
	OutgoingCapacity int            `json:"outgoing_capacity"`
	Incoming         []TypedPayload `json:"incoming"`
	Outgoing         []TypedPayload `json:"outgoing"`
}

// SaveCheckpoint writes the port's buffer capacities and contents. Message types
// must be registered with RegisterMsg.
func (p *defaultPort) SaveCheckpoint(w io.Writer) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	incoming, err := encodeMsgs(p.incomingBuf.Elements())
	if err != nil {
		return fmt.Errorf("messaging: port %q incoming: %w", p.name, err)
	}
	outgoing, err := encodeMsgs(p.outgoingBuf.Elements())
	if err != nil {
		return fmt.Errorf("messaging: port %q outgoing: %w", p.name, err)
	}

	return json.NewEncoder(w).Encode(portCheckpoint{
		IncomingCapacity: p.incomingBuf.Capacity(),
		OutgoingCapacity: p.outgoingBuf.Capacity(),
		Incoming:         incoming,
		Outgoing:         outgoing,
	})
}

// LoadCheckpoint restores the buffer contents after checking that the rebuilt
// port has the saved capacities. Buffers are restored directly, without calling
// Send/Deliver, so no hooks fire and no connection is notified.
func (p *defaultPort) LoadCheckpoint(r io.Reader) error {
	var dto portCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if got := p.incomingBuf.Capacity(); dto.IncomingCapacity != got {
		return fmt.Errorf(
			"messaging: port %q incoming capacity mismatch: checkpoint %d, rebuilt %d",
			p.name, dto.IncomingCapacity, got)
	}
	if got := p.outgoingBuf.Capacity(); dto.OutgoingCapacity != got {
		return fmt.Errorf(
			"messaging: port %q outgoing capacity mismatch: checkpoint %d, rebuilt %d",
			p.name, dto.OutgoingCapacity, got)
	}

	incoming, err := decodeMsgs(dto.Incoming)
	if err != nil {
		return fmt.Errorf("messaging: port %q incoming: %w", p.name, err)
	}
	outgoing, err := decodeMsgs(dto.Outgoing)
	if err != nil {
		return fmt.Errorf("messaging: port %q outgoing: %w", p.name, err)
	}

	p.incomingBuf.Restore(incoming)
	p.outgoingBuf.Restore(outgoing)

	return nil
}

func encodeMsgs(msgs []Msg) ([]TypedPayload, error) {
	out := make([]TypedPayload, len(msgs))
	for i, m := range msgs {
		tp, err := EncodeMsg(m)
		if err != nil {
			return nil, err
		}
		out[i] = tp
	}

	return out, nil
}

func decodeMsgs(payloads []TypedPayload) ([]Msg, error) {
	out := make([]Msg, len(payloads))
	for i, tp := range payloads {
		m, err := DecodeMsg(tp)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}

	return out, nil
}
