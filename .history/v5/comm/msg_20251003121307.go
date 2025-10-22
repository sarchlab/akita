// Package commv5 defines communication primitives for Akita V5.
package commv5

import (
	"reflect"

	"github.com/rs/xid"
)

// PortAddr identifies a port in the simulation topology.
type PortAddr string

// Metadata carries the envelope information shared by all messages.
type Metadata struct {
	ID           string
	Src          PortAddr
	Dst          PortAddr
	TrafficClass string
	TrafficBytes int
}

// Message is a pure-data message envelope used across the simulator.
type Message struct {
	Metadata Metadata
}

// NewMessage constructs a new Message. If meta.ID is empty a fresh ID is
// generated.
func NewMessage(meta Metadata) Message {
	if meta.ID == "" {
		meta.ID = xid.New().String()
	}

	return Message{Metadata: meta}
}

// Clone returns a copy of the message with a freshly generated ID.
func (m Message) Clone() Message {
	clone := m
	clone.Metadata = CloneMetadata(m.Metadata)

	return clone
}

// CloneMetadata returns a copy of the metadata with a freshly generated ID.
func CloneMetadata(meta Metadata) Metadata {
	clone := meta
	clone.ID = xid.New().String()

	return clone
}

// MetadataFor returns Metadata populated for the provided request/response
// type. The TrafficClass is filled with the concrete type name of sample
// while the ID is freshly generated.
func MetadataFor(sample any, src, dst PortAddr, trafficBytes int) Metadata {
	return Metadata{
		ID:           xid.New().String(),
		Src:          src,
		Dst:          dst,
		TrafficClass: typeName(sample),
		TrafficBytes: trafficBytes,
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
