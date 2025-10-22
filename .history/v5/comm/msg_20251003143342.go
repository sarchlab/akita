// Package comm defines communication primitives for Akita V5.
package comm

import (
	"strconv"
	"sync"

	"github.com/sarchlab/akita/v4/v5/idgen"
)

var (
	idGenMu sync.RWMutex
	idGen   *idgen.Generator = idgen.New()
)

// SetIDGenerator overrides the generator used to assign new message IDs. The
// provided generator must be safe for concurrent use.
func SetIDGenerator(g *idgen.Generator) {
	if g == nil {
		panic("id generator cannot be nil")
	}

	idGenMu.Lock()
	idGen = g
	idGenMu.Unlock()
}

func nextID() string {
	idGenMu.RLock()
	g := idGen
	idGenMu.RUnlock()

	return strconv.FormatUint(uint64(g.Generate()), 10)
}

// PortAddr identifies a port in the simulation topology.
type PortAddr string

// MsgMetaData carries the envelope information shared by all messages.
type MsgMetaData struct {
	ID           string
	Src          PortAddr
	Dst          PortAddr
	TrafficClass string
	TrafficBytes int
}

// TransferUnit is a pure-data message envelope used across the simulator. The
// carried message can be any user-defined type and typically points to a struct
// to avoid copying large values.
type TransferUnit struct {
	Metadata MsgMetaData
	Msg      any
}
