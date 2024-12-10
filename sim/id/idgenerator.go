package id

import (
	"log"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rs/xid"
)

var idGeneratorMutex sync.Mutex
var idGeneratorInstantiated bool
var gen idGenerator

type idGenerator interface {
	Generate() string
}

// UseSequentialIDGenerator configures the ID generator to generate IDs in
// sequential.
func UseSequentialIDGenerator() {
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGeneratorMutex.Lock()
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	gen = &sequentialIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// UseParallelIDGenerator configures the ID generator to generate ID in
// parallel. The IDs generated will not be deterministic anymore.
func UseParallelIDGenerator() {
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGeneratorMutex.Lock()
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	gen = &parallelIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// Generate generates an ID that is unique in the current simulation.
func Generate() string {
	return getIDGenerator().Generate()
}

// getIDGenerator returns the ID generator used in the current simulation.
func getIDGenerator() idGenerator {
	if idGeneratorInstantiated {
		return gen
	}

	idGeneratorMutex.Lock()
	if idGeneratorInstantiated {
		idGeneratorMutex.Unlock()
		return gen
	}

	gen = &sequentialIDGenerator{}
	idGeneratorInstantiated = true
	idGeneratorMutex.Unlock()

	return gen
}

type sequentialIDGenerator struct {
	nextID uint64
}

func (g *sequentialIDGenerator) Generate() string {
	idNumber := atomic.AddUint64(&g.nextID, 1)
	id := strconv.FormatUint(idNumber, 10)

	return id
}

type parallelIDGenerator struct {
}

func (g parallelIDGenerator) Generate() string {
	return xid.New().String()
}
