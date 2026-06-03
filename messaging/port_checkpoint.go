package messaging

import (
	"encoding/json"
	"fmt"
	"io"
)

// portCheckpoint is the serialized form of a port. The foundation supports
// quiescent checkpoints only, so it carries the buffer capacities for a shape
// check; the buffers themselves must be empty (serializing in-flight messages
// needs the message codec registry, a later milestone).
type portCheckpoint struct {
	IncomingCapacity int `json:"incoming_capacity"`
	OutgoingCapacity int `json:"outgoing_capacity"`
}

// SaveCheckpoint writes the port's buffer capacities. It refuses to checkpoint a
// port whose buffers are not empty, since restoring in-flight messages is not
// yet supported. Hooks and the owning component/connection are rebuilt by setup
// and not serialized.
func (p *defaultPort) SaveCheckpoint(w io.Writer) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.incomingBuf.Size() != 0 || p.outgoingBuf.Size() != 0 {
		return fmt.Errorf(
			"messaging: cannot checkpoint port %q with non-empty buffers "+
				"(non-quiescent checkpoints are not yet supported)", p.name)
	}

	return json.NewEncoder(w).Encode(portCheckpoint{
		IncomingCapacity: p.incomingBuf.Capacity(),
		OutgoingCapacity: p.outgoingBuf.Capacity(),
	})
}

// LoadCheckpoint verifies that the rebuilt port has the saved buffer capacities
// and empty buffers. There is nothing to restore for a quiescent port.
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
	if p.incomingBuf.Size() != 0 || p.outgoingBuf.Size() != 0 {
		return fmt.Errorf(
			"messaging: cannot load a checkpoint into port %q with non-empty buffers",
			p.name)
	}

	return nil
}
