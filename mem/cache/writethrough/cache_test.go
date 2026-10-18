package writethrough_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	. "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := twowaybuffered.NewPort(bufSize, bufSize)
	p.BindOwner(testDriver{}, name)

	return p
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestCache(t *testing.T) {
	var (
		engine              timing.Engine
		sim                 timing.Simulation
		connection          messaging.Connection
		addressToPortMapper messaging.RemotePort
		dram                *idealmemcontroller.Comp
		dramStorage         *mem.Storage
		cuPort              messaging.Port
		c                   *Comp
	)

	// drainResponses retrieves every message that has been delivered to cuPort.
	drainResponses := func() []messaging.Msg {
		msgs := []messaging.Msg{}
		for {
			msg, ok := cuPort.RetrieveIncoming()
			if !ok {
				break
			}
			msgs = append(msgs, msg)
		}
		return msgs
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		connection = direct.NewConnection("Conn", sim, timing.GHz)

		// cuPort is a real, component-less port that stands in for the compute
		// unit. It is plugged into the connection so the cache's responses land
		// in its incoming buffer, which the tests then drain and inspect.
		cuPort = newDriverPort("CU.Top", 16)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dram = modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
			WithSimulation(sim).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			Build("DRAM")

		dram.BindPort("Top", twowaybuffered.NewPort(16, 16))
		dram.BindPort("Control", twowaybuffered.NewPort(16, 16))

		addressToPortMapper = dram.Ports.Top.AsRemote()

		// The system builder chooses the port buffer sizes. Control is
		// unused here but every port must still be given.
		routingSpec1 := DefaultSpec()

		routingSpec1.Bottom.Destinations = []messaging.RemotePort{addressToPortMapper}
		c = modeling.MakeBuilder[*Comp](routingSpec1).
			WithSimulation(sim).
			WithResources(Resources{Storage: mem.NewStorage(DefaultSpec().TotalByteSize)}).
			Build("Cache")

		c.BindPort("Top", twowaybuffered.NewPort(4, 4))
		c.BindPort("Bottom", twowaybuffered.NewPort(4, 4))
		c.BindPort("Control", twowaybuffered.NewPort(4, 4))

		connection.BindPort(dram.Ports.Top)
		connection.BindPort(c.Ports.Top)
		connection.BindPort(c.Ports.Bottom)
		connection.BindPort(cuPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	t.Run("should do read miss", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		dr := rsps[0]
		require.Equal(t, []byte{1, 2, 3, 4}, dr.Payload.(memprotocol.DataReadyRsp).Data)
	})

	t.Run("should do read miss coalesce", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read1)

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read2)

		require.NoError(t, engine.Run())

		// Without coalescing, the MSHR-hit transaction (read2) may be
		// finalized before the fetcher (read1). Accept responses in
		// any order as long as both data values are received.
		received := make(map[string]bool)
		for _, msg := range drainResponses() {
			dr := msg
			if string(dr.Payload.(memprotocol.DataReadyRsp).Data) == string([]byte{1, 2, 3, 4}) {
				received["1234"] = true
			} else if string(dr.Payload.(memprotocol.DataReadyRsp).Data) == string([]byte{5, 6, 7, 8}) {
				received["5678"] = true
			}
		}

		require.True(t, received["1234"])
		require.True(t, received["5678"])
	})

	t.Run("should do read hit", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read1)
		require.NoError(t, engine.Run())
		t1 := engine.CurrentTime()

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, []byte{1, 2, 3, 4}, rsps[0].Payload.(memprotocol.DataReadyRsp).Data)

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read2)
		require.NoError(t, engine.Run())
		t2 := engine.CurrentTime()

		rsps = drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, []byte{5, 6, 7, 8}, rsps[0].Payload.(memprotocol.DataReadyRsp).Data)

		require.True(t, (t2-t1) < (t1), "expected t2 - t1 < t1")
	})

	t.Run("should write partial line", func(t *testing.T) {
		setup1(t)

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100,
			Data:    []byte{1, 2, 3, 4}},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 4 + 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(write)

		require.NoError(t, engine.Run())

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, write.ID, rsps[0].RspTo)

		data := dramStorage.Read(0x100, 4)
		require.Equal(t, []byte{1, 2, 3, 4}, data)
	})

	t.Run("should write full line", func(t *testing.T) {
		setup1(t)

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100,
			Data: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 64 + 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(write)
		require.NoError(t, engine.Run())

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, write.ID, rsps[0].RspTo)

		data := dramStorage.Read(0x100, 4)
		require.Equal(t, []byte{1, 2, 3, 4}, data)
	})
}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
