package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the writeback cache, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		NumReqPerCycle:      1,
		Log2BlockSize:       6,
		BankLatency:         10,
		WayAssociativity:    4,
		NumBanks:            1,
		NumMSHREntry:        16,
		TotalByteSize:       512 * mem.KB,
		WriteBufferCapacity: 1024,
		MaxInflightFetch:    128,
		MaxInflightEviction: 128,
		InterleavingSize:    4096,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState returns a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func newState(c *Comp) state {
	spec := c.Spec

	numBanks := spec.numBanks()
	laneWidth := spec.laneWidth()

	dirToBank := make([]queueing.Buffer[int], numBanks)
	wbToBank := make([]queueing.Buffer[int], numBanks)
	bankPipes := make([]queueing.Pipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		wbToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		bankPipes[i] = queueing.MakePipeline[int](laneWidth, spec.BankLatency)
		bankPostBufs[i] = queueing.MakeBuffer[int](laneWidth)
	}

	s := state{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf:  queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf:          queueing.MakeBuffer[int](spec.NumReqPerCycle),

		WriteBufferBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirPipeline:        queueing.MakePipeline[int](laneWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankPipelines:                   bankPipes,
		BankPostPipelineBufs:            bankPostBufs,
		BankInflightTransCounts:         make([]int, numBanks),
		BankDownwardInflightTransCounts: make([]int, numBanks),
	}

	cache.DirectoryReset(&s.DirectoryState,
		spec.numSets(), spec.WayAssociativity, 1<<spec.Log2BlockSize)

	return s
}

// newMiddlewares creates the cache's middlewares. The control middlewares and
// the pipeline stages share the pipeline middleware, which holds the storage,
// the address mapper, and the ports' accessors.
func newMiddlewares(c *Comp) middlewares {
	res := c.Resources
	if res.Storage == nil {
		panic("writeback: Resources.Storage is required")
	}

	pipeline := &pipelineMW{
		comp:          c,
		storage:       res.Storage,
		addressMapper: resolveAddressMapper(c.Spec, res),
	}
	pipeline.createInternalStages()

	return middlewares{
		Ctrl: &ctrlMiddleware{pipeline: pipeline},
		Flusher: &controlMW{
			comp:    c,
			flusher: &flusher{pipeline: pipeline},
		},
		Pipeline: pipeline,
	}
}

// resolveAddressMapper returns the mapper that routes fetches and evictions
// to lower memory: the injected Resources.AddressToPortMapper, or one built
// from Spec.AddressMapperType over Resources.RemotePorts. It returns nil when
// neither is configured or there are no remote ports, so the first routed
// request reports the missing wiring.
func resolveAddressMapper(spec Spec, res Resources) mem.AddressToPortMapper {
	if res.AddressToPortMapper != nil {
		return res.AddressToPortMapper
	}

	ports := res.RemotePorts

	switch spec.AddressMapperType {
	case "":
		return nil
	case "single":
		if len(ports) == 0 {
			return nil
		}

		return &mem.SinglePortMapper{Port: ports[0]}
	case "interleaved":
		if len(ports) == 0 {
			return nil
		}

		if spec.InterleavingSize == 0 {
			panic("writeback: an interleaved address mapper needs a " +
				"non-zero Spec.InterleavingSize")
		}

		return &mem.InterleavedAddressPortMapper{
			InterleavingSize: spec.InterleavingSize,
			LowModules:       append([]messaging.RemotePort(nil), ports...),
		}
	default:
		panic(fmt.Sprintf(
			"writeback: unknown address mapper type %q",
			spec.AddressMapperType))
	}
}
