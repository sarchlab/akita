// Package idgen provides deterministic-friendly ID generators for V5.
package idgen

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rs/xid"
)

// ID is a unique identifier for messages and other entities in Akita V5.
type ID string

// Generator produces unique string identifiers.
type Generator interface {
	Generate() ID
}

// Option configures how the default generator behaves.
type Option func(*factory)

// WithParallel turns the generator into a non-deterministic, parallel-safe
// implementation backed by xid.
func WithParallel() Option {
	return func(f *factory) {
		f.new = func() Generator { return parallelGenerator{} }
	}
}

// WithStart sets the starting value for the sequential generator.
func WithStart(start uint64) Option {
	return func(f *factory) {
		if start == 0 {
			return
		}
		f.new = func() Generator {
			return &sequentialGenerator{next: start}
		}
	}
}

var defaultFactory = &factory{new: func() Generator { return &sequentialGenerator{} }}
var defaultOnce sync.Once

// Default returns the process-wide default generator. The first call can pass
// options to configure it; later calls ignore options to ensure determinism.
func Default(opts ...Option) Generator {
	defaultOnce.Do(func() {
		f := &factory{new: func() Generator { return &sequentialGenerator{} }}
		for _, opt := range opts {
			opt(f)
		}
		defaultFactory = f
	})

	return defaultFactory.get()
}

type factory struct {
	mu  sync.Mutex
	gen Generator
	new func() Generator
}

func (f *factory) get() Generator {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.gen == nil {
		f.gen = f.new()
	}

	return f.gen
}

type sequentialGenerator struct {
	next uint64
}

func (g *sequentialGenerator) Generate() string {
	id := atomic.AddUint64(&g.next, 1)
	return strconv.FormatUint(id, 10)
}

type parallelGenerator struct{}

func (parallelGenerator) Generate() string {
	return xid.New().String()
}

// Reset clears the default generator so subsequent calls to Default may be
// reconfigured. Intended for tests only.
func Reset() {
	defaultFactory = &factory{new: func() Generator { return &sequentialGenerator{} }}
	defaultOnce = sync.Once{}
}
