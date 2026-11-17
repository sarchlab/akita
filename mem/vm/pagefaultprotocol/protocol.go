// Package pagefaultprotocol carries fallback translations and placement decisions
// between memory-backed MMUs and a platform's fault handler.
package pagefaultprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "requester", Sends: []any{Req{}}},
		messaging.RoleDef{Name: "responder", Sends: []any{Rsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// Req preserves the original virtual address and requesting device at every hop.
// Mapping is meaningful only when HasMapping is set.
type Req struct {
	PID        vm.PID
	VAddr      uint64
	DeviceID   uint64
	HasMapping bool
	Mapping    vm.Page
}

// Result is the handler's decision for this transaction.
type Result uint8

const (
	Resolved Result = iota + 1 // Mapping may be used, including a remote mapping.
	Blocked                    // NACK now; the requester translates again after the driver releases it.
)

// Rsp answers one Req through messaging.Msg.RspTo. Resolved supplies a mapping
// already safe for the requester to use. An MMU drains outstanding fault
// decisions before acknowledging a control command covering the page. A handler
// must answer the fault before waiting for that ACK. When installation or
// migration is needed, reply Blocked, coordinate Hold and mapping updates,
// then Release the requester to translate again.
type Rsp struct {
	Result  Result
	Mapping vm.Page
}
