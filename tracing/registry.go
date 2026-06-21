package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// The receiver task ID registry maps (domain, message-ID) to the local task ID
// that the receiver uses to track its handling of that message. This lets a
// receiver derive a stable task ID for an incoming message without mutating
// the message itself.

type receiverTaskKey struct {
	domain string
	msgID  uint64
}

var (
	receiverTaskIDs   = make(map[receiverTaskKey]uint64)
	receiverTaskIDsMu sync.Mutex
)

func lookupOrCreateReceiverTaskID(msg messaging.Msg, domain NamedHookable) uint64 {
	key := receiverTaskKey{domain: domain.Name(), msgID: msg.Meta().ID}

	receiverTaskIDsMu.Lock()
	defer receiverTaskIDsMu.Unlock()

	if id, ok := receiverTaskIDs[key]; ok {
		return id
	}

	id := timing.GetIDGenerator().Generate()
	receiverTaskIDs[key] = id

	return id
}

// receiverTaskIDByMsgID returns the receiver-side task ID registered for the
// message ID at the domain, and whether one exists. Unlike
// lookupOrCreateReceiverTaskID it never creates an entry, so a reset path can
// resolve an in-flight task's ID to end it without resurrecting an entry that
// was already forgotten.
func receiverTaskIDByMsgID(
	msgID uint64, domain NamedHookable,
) (uint64, bool) {
	key := receiverTaskKey{domain: domain.Name(), msgID: msgID}

	receiverTaskIDsMu.Lock()
	defer receiverTaskIDsMu.Unlock()

	id, ok := receiverTaskIDs[key]

	return id, ok
}

func forgetReceiverTaskID(msg messaging.Msg, domain NamedHookable) {
	forgetReceiverTaskIDByMsgID(msg.Meta().ID, domain)
}

func forgetReceiverTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	key := receiverTaskKey{domain: domain.Name(), msgID: msgID}

	receiverTaskIDsMu.Lock()
	delete(receiverTaskIDs, key)
	receiverTaskIDsMu.Unlock()
}

// The incoming-buffer task ID registry maps (domain, message-ID) to the task ID
// of the buffer task that tracks a message's residency in a port's incoming
// buffer (from delivery until it is retrieved). The port hook that opens the
// task and the component that hangs admission milestones on it both derive the
// same ID from the message, without mutating the message.

type incomingBufferTaskKey struct {
	domain string
	msgID  uint64
}

var (
	incomingBufferTaskIDs   = make(map[incomingBufferTaskKey]uint64)
	incomingBufferTaskIDsMu sync.Mutex
)

func lookupOrCreateIncomingBufferTaskID(
	msg messaging.Msg, domain NamedHookable,
) uint64 {
	key := incomingBufferTaskKey{domain: domain.Name(), msgID: msg.Meta().ID}

	incomingBufferTaskIDsMu.Lock()
	defer incomingBufferTaskIDsMu.Unlock()

	if id, ok := incomingBufferTaskIDs[key]; ok {
		return id
	}

	id := timing.GetIDGenerator().Generate()
	incomingBufferTaskIDs[key] = id

	return id
}

func forgetIncomingBufferTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	key := incomingBufferTaskKey{domain: domain.Name(), msgID: msgID}

	incomingBufferTaskIDsMu.Lock()
	delete(incomingBufferTaskIDs, key)
	incomingBufferTaskIDsMu.Unlock()
}
