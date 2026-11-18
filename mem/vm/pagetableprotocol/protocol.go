// Package pagetableprotocol defines commands that update a memory-backed MMU.
package pagetableprotocol

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

// Command selects the affected range and the operation.
type Command uint8

const (
	RegisterRoot Command = iota + 1 // Register and zero Root, a free frame in this MMU's pool.
	Map                             // Insert a previously absent page.
	Remap                           // Replace an existing page.
	Unmap                           // Remove an existing leaf, retaining its intermediate nodes.
	Invalidate                      // Invalidate walk-cache entries covering [VAddr, VAddr+Size).
	Teardown                        // Release the root and every intermediate node owned by PID.
)

// Req addresses a PID. VAddr names a page for Map, Remap and Unmap, and
// the start of a page-aligned byte range for Invalidate. Map and Remap use
// Mapping's PAddr, DeviceID and policy flags; PID, VAddr, size and validity
// are determined by the command and the component geometry.
type Req struct {
	Command Command
	PID     vm.PID
	Root    uint64
	VAddr   uint64
	Size    uint64
	Mapping vm.Page
}

// Rsp acknowledges a committed command. A nonempty Error rejects it.
type Rsp struct{ Error string }
