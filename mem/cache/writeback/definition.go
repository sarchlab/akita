package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the writeback cache, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
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
func newState(c *Comp) State {
	name, spec := c.Name(), c.Spec()

	numBanks := spec.numBanks()
	laneWidth := spec.laneWidth()

	dirToBank := make([]queueing.Buffer[int], numBanks)
	wbToBank := make([]queueing.Buffer[int], numBanks)
	bankPipes := make([]queueing.Pipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.DirToBankBuf%d", name, i), spec.NumReqPerCycle)
		wbToBank[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.WriteBufferToBankBuf%d", name, i),
			spec.NumReqPerCycle)
		bankPipes[i] = queueing.NewPipeline[int](laneWidth, spec.BankLatency)
		bankPostBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.BankPostPipelineBuf%d", name, i), laneWidth)
	}

	s := State{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf: queueing.NewBuffer[int](
			name+".DirStageBuf", spec.NumReqPerCycle),
		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf: queueing.NewBuffer[int](
			name+".MSHRStageBuf", spec.NumReqPerCycle),
		WriteBufferBuf: queueing.NewBuffer[int](
			name+".WriteBufferBuf", spec.NumReqPerCycle),
		DirPipeline: queueing.NewPipeline[int](laneWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.NewBuffer[int](
			name+".DirPostPipelineBuf", spec.NumReqPerCycle),
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
func newMiddlewares(c *Comp) Middlewares {
	res := c.Resources()
	if res.Storage == nil {
		panic("writeback: Resources.Storage is required")
	}

	pipeline := &pipelineMW{
		comp:          c,
		storage:       res.Storage,
		addressMapper: resolveAddressMapper(c.Spec(), res),
	}
	pipeline.createInternalStages()

	return Middlewares{
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

		mapper := mem.NewInterleavedAddressPortMapper(spec.InterleavingSize)
		mapper.LowModules = append(mapper.LowModules, ports...)

		return mapper
	default:
		panic(fmt.Sprintf(
			"writeback: unknown address mapper type %q",
			spec.AddressMapperType))
	}
}
