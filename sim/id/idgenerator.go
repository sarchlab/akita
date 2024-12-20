package id

import (
	"strconv"
	"sync/atomic"
)

type IDGenerator interface {
	Generate() string
}

// NewIDGenerator returns the ID generator used in the current simulation.
func NewIDGenerator() IDGenerator {
	return &sequentialIDGenerator{}
}

type sequentialIDGenerator struct {
	nextID uint64
}

func (g *sequentialIDGenerator) Generate() string {
	idNumber := atomic.AddUint64(&g.nextID, 1)
	id := strconv.FormatUint(idNumber, 10)

	return id
}

// type parallelIDGenerator struct {
// }

// func (g parallelIDGenerator) Generate() string {
// 	return xid.New().String()
// }
