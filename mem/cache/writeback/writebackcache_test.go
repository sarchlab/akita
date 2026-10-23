package writeback

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/messaging Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/timing Engine

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// makePorts creates the ports of the writeback cache named name, each with
// bufSize slots in each direction.
func makePorts(simulation modeling.Simulation, name string, bufSize int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
		Bottom: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
	}
}

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(simulation modeling.Simulation, name string, bufSize int) messaging.Port {
	p := twowaybuffered.MakeBuilder().
		WithSimulation(simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
		Build()
	p.BindOwner(testDriver{}, name)

	simulation.RegisterPort(p)
	return p
}

// plugNoopConn plugs each of comp's ports into a noop connection, so a test
// can drive the ports directly.
func plugNoopConn(comp *Comp) {
	for _, p := range []messaging.Port{
		comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
	} {
		(&ccNoopConn{}).BindPort(p)
	}
}

// stageTestSpec returns the configuration the white-box stage tests share:
// 64 sets of 4 ways of 64-byte blocks in a single bank, 4 requests per cycle.
func stageTestSpec() Spec {
	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
	spec.Log2BlockSize = 6
	spec.NumWays = 4
	spec.NumSets = 64
	spec.NumBanks = 1
	spec.NumReqPerCycle = 4

	spec.DirectoryWidth = 4
	spec.BankWidth = 4
	return spec
}

// buildStageTestComp builds the "Cache" instance a white-box stage test
// drives, with its ports plugged into noop connections. The test ticks the
// stage under test directly rather than the component.
func buildStageTestComp(setupSim1 *akitasim.Simulation, spec Spec, res Resources, ports Ports) *Comp {
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(setupSim1).
		WithResources(res).
		Build("Cache")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	plugNoopConn(comp)
	if err := setupSim1.Initialize(); err != nil {
		panic(err)
	}

	return comp
}

// buildIdealDRAM builds a 200-cycle ideal memory controller named "DRAM",
// backed by storage, and returns its Top port.
func buildIdealDRAM(sim *testsim.Simulation, storage *mem.Storage) messaging.Port {
	dramSpec := idealmemcontroller.DefaultSpec()
	dramSpec.Width = 1
	dramSpec.Latency = 200
	dram := modeling.MakeBuilder[*idealmemcontroller.Comp](dramSpec).
		WithSimulation(sim).
		WithResources(idealmemcontroller.Resources{Storage: storage}).
		Build("DRAM")

	dram.BindPort("Top", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())
	dram.BindPort("Control", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())

	return dram.Ports.Top
}

func TestWriteBackCacheIntegration(t *testing.T) {
	var (
		engine              timing.Engine
		sim                 *testsim.Simulation
		addressToPortMapper messaging.RemotePort
		cacheComp           *Comp
		m                   *pipelineMW
		dramTop             messaging.Port
		dramStorage         *mem.Storage
		conn                *direct.Connection
		agentPort           messaging.Port
		controlAgentPort    messaging.Port
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		agentPort = newDriverPort(sim, "Agent.Top", 8)
		controlAgentPort = newDriverPort(sim, "Agent.Control", 8)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramTop = buildIdealDRAM(sim, dramStorage)

		addressToPortMapper = dramTop.AsRemote()

		cacheSpec := DefaultSpec()
		cacheSpec.NumSets = 1024
		cacheSpec.NumReqPerCycle = 4

		cacheSpec.DirectoryWidth = 4
		cacheSpec.BankWidth = 4
		cacheSpec.Bottom.Destinations = []messaging.RemotePort{addressToPortMapper}
		cacheComp = modeling.MakeBuilder[*Comp](cacheSpec).
			WithSimulation(sim).
			WithResources(Resources{Storage: mem.NewStorage(cacheSpec.ByteSize())}).
			Build("Cache")
		cacheCompPorts := makePorts(sim, "Cache", 8)
		cacheComp.BindPort("Top", cacheCompPorts.Top)
		cacheComp.BindPort("Bottom", cacheCompPorts.Bottom)
		cacheComp.BindPort("Control", cacheCompPorts.Control)

		conn = direct.NewConnection("Connection", sim, timing.GHz)
		conn.BindPort(cacheComp.Ports.Top)
		conn.BindPort(cacheComp.Ports.Bottom)
		conn.BindPort(cacheComp.Ports.Control)
		conn.BindPort(dramTop)
		conn.BindPort(agentPort)
		conn.BindPort(controlAgentPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
		m = cacheComp.Middlewares.Pipeline
	}

	t.Run("should do read hit", func(t *testing.T) {
		setup1(t)

		state := m.comp.State
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.NumSets)
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.State = state
		m.storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		require.Equal(t, read.ID, dr.RspTo)
	})

	t.Run("should write hit", func(t *testing.T) {
		setup1(t)

		state := m.comp.State
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.NumSets)
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.State = state
		m.storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x10004,
			Data:    []byte{9, 9, 9, 9}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{9, 9, 9, 9}) + 12,
		}

		cacheComp.Ports.Top.Deliver(write)

		require.NoError(t, engine.Run())

		rsp, _ := agentPort.RetrieveIncoming()
		require.Equal(t, write.ID, rsp.RspTo)

		// Re-read state after engine run
		postState := m.comp.State
		postBlock := &postState.DirectoryState.Sets[setID].Blocks[0]
		retData := m.storage.Read(postBlock.CacheAddress+0x4, 4)
		require.Equal(t, write.Payload.(memprotocol.WriteReq).Data, retData)
		require.True(t, postBlock.IsValid)
		require.True(t, postBlock.IsDirty)
	})

	t.Run("should do read miss, mshr miss, w/ fetch, w/o eviction", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		require.Equal(t, read.ID, dr.RspTo)
	})

	t.Run("should handle read miss, mshr hit", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read1)

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10008,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read2)

		require.NoError(t, engine.Run())

		rsps := map[uint64][]byte{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			require.NotNil(t, rsp)
			dr := rsp
			rsps[dr.RspTo] = dr.Payload.(memprotocol.DataReadyRsp).Data
		}
		require.Equal(t, []byte{5, 6, 7, 8}, rsps[read1.ID])
		require.Equal(t, []byte{1, 2, 3, 4}, rsps[read2.ID])
	})

	t.Run("should handle write miss, mshr miss, w/o fetch, w/o eviction", func(t *testing.T) {
		setup1(t)

		writeData := []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x10000,
			Data:    writeData},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len(writeData) + 12,
		}

		cacheComp.Ports.Top.Deliver(write)

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsps := map[uint64]messaging.Msg{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			require.NotNil(t, rsp)
			rsps[rsp.RspTo] = rsp
		}
		require.Contains(t, rsps, write.ID)
		dr := rsps[read.ID]
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
	})

	t.Run("should handle read miss, mshr miss, w/ fetch, w/ eviction", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		// Fill target set with dirty blocks
		state := m.comp.State
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.NumSets)
		for i := 0; i < spec.NumWays; i++ {
			block := &state.DirectoryState.Sets[setID].Blocks[i]
			block.IsValid = true
			block.IsDirty = true
		}
		m.comp.State = state

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
		}

		cacheComp.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		require.Equal(t, read.ID, dr.RspTo)
	})

	t.Run("should flush", func(t *testing.T) {
		setup1(t)

		write1 := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100000,
			Data:    []byte{1, 2, 3, 4}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{1, 2, 3, 4}) + 12,
		}

		cacheComp.Ports.Top.Deliver(write1)

		write2 := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100000,
			Data:    []byte{1, 2, 3, 4}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{1, 2, 3, 4}) + 12,
		}

		cacheComp.Ports.Top.Deliver(write2)

		// Let the writes settle so the block is resident and dirty.
		require.NoError(t, engine.Run())
		_, present0 := controlAgentPort.RetrieveIncoming()
		require.False(t, present0)
		// Flush is a conditional verb: pause first so it is legal.
		pause := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdPause},
			ID:  m.comp.NewID(),
			Src: controlAgentPort.AsRemote(),
			Dst: cacheComp.Ports.Control.AsRemote(),
		}

		cacheComp.Ports.Control.Deliver(pause)

		require.NoError(t, engine.Run())

		pauseRsp, _ := controlAgentPort.RetrieveIncoming()
		require.NotNil(t, pauseRsp)
		require.Equal(t, memcontrolprotocol.CmdPause, pauseRsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, pauseRsp.Payload.(memcontrolprotocol.Rsp).Success)

		flush := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
			ID:  m.comp.NewID(),
			Src: controlAgentPort.AsRemote(),
			Dst: cacheComp.Ports.Control.AsRemote(),
		}

		cacheComp.Ports.Control.Deliver(flush)

		require.NoError(t, engine.Run())

		rsp, _ := controlAgentPort.RetrieveIncoming()
		require.NotNil(t, rsp)
		require.Equal(t, flush.ID, rsp.RspTo)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)

		// The dirty block's data reached DRAM.
		flushed := dramStorage.Read(0x100000, 4)
		require.Equal(t, []byte{1, 2, 3, 4}, flushed)
	})
}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
