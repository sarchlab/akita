package tracing

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Tracking belongs to the simulation's ID sequence, so identical component
// names and message IDs in two simulations cannot share task identities.
func registry(domain NamedHookable) timing.IDGenerator { return timing.IDsFor(domain) }
func lookupOrCreateReceiverTaskID(msg messaging.Msg, domain NamedHookable) uint64 {
	return registry(domain).Track(domain.Name()+"/receiver", msg.Meta().ID)
}
func receiverTaskIDByMsgID(msgID uint64, domain NamedHookable) (uint64, bool) {
	return registry(domain).Lookup(domain.Name()+"/receiver", msgID)
}
func forgetReceiverTaskID(msg messaging.Msg, domain NamedHookable) {
	forgetReceiverTaskIDByMsgID(msg.Meta().ID, domain)
}
func forgetReceiverTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	registry(domain).Forget(domain.Name()+"/receiver", msgID)
}
func lookupOrCreateIncomingBufferTaskID(msg messaging.Msg, domain NamedHookable) uint64 {
	return registry(domain).Track(domain.Name()+"/incoming", msg.Meta().ID)
}
func forgetIncomingBufferTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	registry(domain).Forget(domain.Name()+"/incoming", msgID)
}
func lookupOrCreateOutgoingBufferTaskID(msg messaging.Msg, domain NamedHookable) uint64 {
	return registry(domain).Track(domain.Name()+"/outgoing", msg.Meta().ID)
}
func forgetOutgoingBufferTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	registry(domain).Forget(domain.Name()+"/outgoing", msgID)
}
