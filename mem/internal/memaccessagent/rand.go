package memaccessagent

import "math/rand"

// randState is the agent's random source, kept in State so that it is saved
// in checkpoints. It draws the same values as
// rand.New(rand.NewSource(Seed)). Only Seed and the number of values drawn so
// far are saved; the generator is rebuilt from them on first use, for example
// after a checkpoint is loaded.
type randState struct {
	Seed  int64  `json:"seed"`
	Draws uint64 `json:"draws"`

	// src caches the generator. It is rebuilt from Seed and Draws.
	src rand.Source64 `json:"-"`
}

func newRandState(seed int64) randState {
	return randState{Seed: seed}
}

// source returns the generator, rebuilding it when needed by seeding a new
// one and advancing it past the values already drawn.
func (r *randState) source() rand.Source64 {
	if r.src == nil {
		r.src = rand.NewSource(r.Seed).(rand.Source64)
		for range r.Draws {
			r.src.Uint64()
		}
	}

	return r.src
}

func (r *randState) int63() int64 {
	v := r.source().Int63()
	r.Draws++

	return v
}

// Uint64 returns a random uint64, as rand.Rand.Uint64 does.
func (r *randState) Uint64() uint64 {
	v := r.source().Uint64()
	r.Draws++

	return v
}

// Uint32 returns a random uint32, as rand.Rand.Uint32 does.
func (r *randState) Uint32() uint32 {
	return uint32(r.int63() >> 31)
}

// Float64 returns a random float64 in [0, 1), as rand.Rand.Float64 does.
func (r *randState) Float64() float64 {
	for {
		f := float64(r.int63()) / (1 << 63)
		if f != 1 {
			return f
		}
	}
}
