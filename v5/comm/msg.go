// Package comm defines communication primitives for Akita V5.
package comm

import (
	"reflect"
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

// Msg describes the metadata contract shared by all messages in the
// communication layer. Concrete message structs should embed MsgMeta (or
// otherwise provide the Meta method) and keep their payload fields alongside
// it.
type Msg interface {
	Meta() *MsgMetaData
}

// MsgMeta provides a reusable metadata implementation that can be embedded in
// message structs to satisfy the Msg interface.
type MsgMeta struct {
	Metadata MsgMetaData
}

// NewMsgMeta constructs metadata for a message, leaving responsibility for
// populating derived fields (ID, TrafficClass) to EnsureMeta.
func NewMsgMeta(meta MsgMetaData) MsgMeta {
	return MsgMeta{Metadata: meta}
}

// Meta exposes the underlying metadata struct.
func (m *MsgMeta) Meta() *MsgMetaData { return &m.Metadata }

// EnsureMeta fills in derived metadata fields such as ID and TrafficClass. Call
// this before handing the message to shared infrastructure so identifiers and
// traffic categories are consistent across the system.
func EnsureMeta(msg Msg) {
	meta := msg.Meta()
	if meta == nil {
		panic("comm: Msg.Meta() returned nil metadata")
	}

	if meta.ID == "" {
		meta.ID = nextID()
	}

	if meta.TrafficClass == "" {
		meta.TrafficClass = typeName(msg)
	}
}

func typeName(v any) string {
	if v == nil {
		return ""
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.String()
}
