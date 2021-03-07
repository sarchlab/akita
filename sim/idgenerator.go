package sim

import (
	"log"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rs/xid"
)

var idGeneratorMutex sync.Mutex
var idGeneratorInstantiated bool
var idGenerator IDGenerator

// IDGenerator can generate IDs
type IDGenerator interface {
	// Generate an ID
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

	idGenerator = &sequentialIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// UseParallelIDGenerator configurs the ID generator to generate ID in
// parallel. The IDs generated will not be deterministic anymore.
func UseParallelIDGenerator() {
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGeneratorMutex.Lock()
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGenerator = &parallelIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// GetIDGenerator returns the ID generator used in the current simulation
func GetIDGenerator() IDGenerator {
	if idGeneratorInstantiated {
		return idGenerator
	}

	idGeneratorMutex.Lock()
	if idGeneratorInstantiated {
		idGeneratorMutex.Unlock()
		return idGenerator
	}

	idGenerator = &sequentialIDGenerator{}
	idGeneratorInstantiated = true
	idGeneratorMutex.Unlock()
	return idGenerator
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
