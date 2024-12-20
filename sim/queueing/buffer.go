package queueing

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/simulation"
)

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &hooking.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &hooking.HookPos{Name: "Buf Pop"}

// A Buffer is a fifo queue for anything
type Buffer interface {
	naming.Named
	hooking.Hookable
	simulation.StateHolder

	CanPush() bool
	Push(e interface{})
	Pop() interface{}
	Peek() interface{}
	Capacity() int
	Size() int
	Clear()
}

// BufferBuilder is a builder for Buffer.
type BufferBuilder struct {
	simulation simulation.Simulation
	capacity   int
}

// WithSimulation defines simulation environment of the buffer.
func (b BufferBuilder) WithSimulation(
	sim simulation.Simulation,
) BufferBuilder {
	b.simulation = sim
	return b
}

// WithCapacity defines the capacity of the buffer.
func (b BufferBuilder) WithCapacity(capacity int) BufferBuilder {
	b.capacity = capacity
	return b
}

// Build builds a new Buffer.
func (b BufferBuilder) Build(name string) Buffer {
	buffer := &bufferImpl{
		bufferState: &bufferState{
			name:     name,
			elements: nil,
		},
		capacity: b.capacity,
	}

	b.simulation.RegisterStateHolder(buffer)

	return buffer
}

func init() {
	serialization.RegisterType(reflect.TypeOf(&bufferState{}))
}

type bufferState struct {
	name     string
	elements []interface{}
}

func (s *bufferState) Name() string {
	return s.name
}

func (s *bufferState) Serialize() (map[string]any, error) {
	return map[string]any{
		"elements": s.elements,
	}, nil
}

func (s *bufferState) Deserialize(state map[string]any) error {
	s.elements = state["elements"].([]interface{})

	return nil
}

type bufferImpl struct {
	hooking.HookableBase
	*bufferState

	capacity int
}

// Name returns the name of the buffer.
func (b *bufferImpl) Name() string {
	return b.name
}

// State returns the state of the buffer.
func (b *bufferImpl) State() simulation.State {
	return b.bufferState
}

// SetState sets the state of the buffer.
func (b *bufferImpl) SetState(state simulation.State) {
	b.bufferState = state.(*bufferState)
}

func (b *bufferImpl) CanPush() bool {
	return len(b.elements) < b.capacity
}

func (b *bufferImpl) Push(e interface{}) {
	if len(b.elements) >= b.capacity {
		log.Panic("buffer overflow")
	}

	b.elements = append(b.elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}
}

func (b *bufferImpl) Pop() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	e := b.elements[0]
	b.elements = b.elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}

	return e
}

func (b *bufferImpl) Peek() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	return b.elements[0]
}

func (b *bufferImpl) Capacity() int {
	return b.capacity
}

func (b *bufferImpl) Size() int {
	return len(b.elements)
}

func (b *bufferImpl) Clear() {
	b.elements = nil
}
