package simplebankedmemory

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/simulation/hooking"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/naming"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

type loopbackConnection struct {
	hooking.HookableBase

	name  string
	ports []messaging.Port
}

func newLoopbackConnection(name string) *loopbackConnection {
	return &loopbackConnection{
		name: name,
	}
}

func (c *loopbackConnection) Name() string {
	return c.name
}

func (c *loopbackConnection) PlugIn(port messaging.Port) {
	c.ports = append(c.ports, port)
	port.SetConnection(c)
}

func (c *loopbackConnection) Unplug(messaging.Port) {
	panic("not implemented")
}

func (c *loopbackConnection) NotifyAvailable(messaging.Port) {
	// No-op for the tests.
}

func (c *loopbackConnection) NotifySend() {
	c.transfer()
}

func (c *loopbackConnection) transfer() {
	if len(c.ports) != 2 {
		panic("loopbackConnection expects exactly two ports")
	}

	src := c.ports[0]
	dst := c.ports[1]
	c.forward(src, dst)
	c.forward(dst, src)
}

func (c *loopbackConnection) forward(src, dst messaging.Port) {
	for {
		msg, ok := src.PeekOutgoing()
		if !ok {
			break
		}

		if !dst.CanDeliver() {
			break
		}

		dst.Deliver(msg)
		src.RetrieveOutgoing()
	}
}

type testAgent struct {
	hooking.HookableBase

	name     string
	port     messaging.Port
	received []messaging.Msg
}

func newTestAgent(name string) *testAgent {
	naming.MustBeValid(name)

	a := &testAgent{
		name: name,
	}

	a.port = messaging.NewPort(fmt.Sprintf("%s.Port", name), 4, 4)
	a.port.SetOwner(a)

	return a
}

func (a *testAgent) Name() string {
	return a.name
}

func (a *testAgent) NotifyRecv(port messaging.Port) {
	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			break
		}

		a.received = append(a.received, msg)
	}
}

func (a *testAgent) NotifyPortFree(messaging.Port) {
	// No-op.
}

func (a *testAgent) Handle(timing.Event) {
	return
}

func (a *testAgent) send(msg messaging.Msg) {
	Expect(a.port.CanSend()).To(BeTrue())
	a.port.Send(msg)
}

type bandwidthAgent struct {
	hooking.HookableBase

	name         string
	port         messaging.Port
	completed    int
	completedIDs []uint64
}

func newBandwidthAgent(name string) *bandwidthAgent {
	naming.MustBeValid(name)

	a := &bandwidthAgent{
		name: name,
	}

	a.port = messaging.NewPort(fmt.Sprintf("%s.Port", name), 8, 8)
	a.port.SetOwner(a)

	return a
}

func (a *bandwidthAgent) Name() string {
	return a.name
}

func (a *bandwidthAgent) NotifyRecv(port messaging.Port) {
	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			break
		}

		if msg.IsRsp() {
			a.completed++
			a.completedIDs = append(a.completedIDs, msg.RspTo)
		}
	}
}

func (a *bandwidthAgent) NotifyPortFree(messaging.Port) {}

func (a *bandwidthAgent) Handle(timing.Event) {
	return
}

const (
	numRequests = 100000
	readSize    = 64
)

func setupExampleSystem() (*Comp, *bandwidthAgent, *loopbackConnection, timing.Freq) {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	freq := 1 * timing.GHz

	spec := Definition.DefaultSpec
	spec.Freq = freq
	spec.NumBanks = 16
	spec.StageLatency = 6
	spec.PostPipelineBufSize = 32

	memComp := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(Resources{Storage: mem.NewStorage(spec.Capacity)}).
		WithPorts(makePorts("Mem", 32, 16)).
		Build("Mem")

	topPort := memComp.Ports.Top
	agent := newBandwidthAgent("Agent")
	conn := newLoopbackConnection("Conn")
	conn.PlugIn(topPort)
	conn.PlugIn(agent.port)

	return memComp, agent, conn, freq
}

func makeReadReq(ids interface{ NewID() uint64 }, src, dst messaging.RemotePort, index int) messaging.Msg {
	addr := uint64(index * readSize)
	r := messaging.Msg{Payload: memprotocol.ReadReq{
		Address:        addr,
		AccessByteSize: readSize},
		ID:  ids.NewID(),
		Src: src,
		Dst: dst,

		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	return r
}

func collectLatency(
	agent *bandwidthAgent,
	startCycles map[uint64]int,
	currentCycle int,
	processed *int,
) float64 {
	var latency float64

	for *processed < agent.completed {
		id := agent.completedIDs[*processed]
		latency += float64(currentCycle - startCycles[id])
		delete(startCycles, id)
		(*processed)++
	}

	return latency
}

var _ = Describe("SimpleBankedMemory", func() {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		memComp *Comp
		storage *mem.Storage
		agent   *testAgent
		conn    *loopbackConnection
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		storage = mem.NewStorage(4 * mem.GB)

		spec := Definition.DefaultSpec
		spec.NumBanks = 2
		spec.StageLatency = 2

		memComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{Storage: storage}).
			WithPorts(makePorts("Mem", 4, 16)).
			Build("Mem")

		topPort := memComp.Ports.Top
		agent = newTestAgent("Agent")
		conn = newLoopbackConnection("Conn")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)
	})

	AfterEach(func() {
		agent.received = nil
	})

	It("should return read data after configured latency", func() {
		data := []byte{1, 2, 3, 4}
		storage.Write(0x0, data)

		topPort := memComp.Ports.Top
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x0,
			AccessByteSize: uint64(len(data))},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		agent.send(read)

		for i := 0; i < 6; i++ {
			modelingtest.Tick(memComp)
		}

		Expect(agent.received).To(HaveLen(1))
		rsp := agent.received[0]
		Expect(rsp.Payload.(memprotocol.DataReadyRsp).Data).To(Equal(data))
	})

	It("should commit write before serving subsequent read", func() {
		addr := uint64(0x100)

		initial := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		storage.Write(addr, initial)

		newData := []byte{0x10, 0x20, 0x30, 0x40}

		topPort := memComp.Ports.Top

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: addr,
			Data:    newData},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: len(newData) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: uint64(len(newData))},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		agent.send(write)
		agent.send(read)

		for i := 0; i < 10; i++ {
			modelingtest.Tick(memComp)
		}

		Expect(agent.received).To(HaveLen(2))
		_, isWriteDone := agent.received[0].Payload.(memprotocol.WriteDoneRsp)
		Expect(isWriteDone).To(BeTrue())
		_, ok := agent.received[1].Payload.(memprotocol.DataReadyRsp)
		readRsp := agent.received[1]
		Expect(ok).To(BeTrue())
		Expect(readRsp.Payload.(memprotocol.DataReadyRsp).Data).To(Equal(newData))

		committed := storage.Read(addr, uint64(len(newData)))
		Expect(committed).To(Equal(newData))
	})

	It("accesses storage at the global request address (identity)", func() {
		// Storage is global: a request's address indexes the backing store
		// directly, with no per-controller conversion.
		spec := Definition.DefaultSpec
		spec.NumBanks = 2
		spec.StageLatency = 2

		memComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{Storage: mem.NewStorage(spec.Capacity)}).
			WithPorts(makePorts("MemGlobal", 4, 16)).
			Build("MemGlobal")

		topPort := memComp.Ports.Top
		agent = newTestAgent("AgentGlobal")
		conn = newLoopbackConnection("ConnGlobal")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)

		// Write 4 bytes at a non-zero global address.
		writeData := []byte{1, 2, 3, 4}
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x200,
			Data:    writeData},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: len(writeData) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		// Read the same global address back.
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x200,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		agent.send(write)
		agent.send(read)

		for i := 0; i < 12; i++ {
			modelingtest.Tick(memComp)
		}

		Expect(agent.received).To(HaveLen(2))
		_, ok := agent.received[1].Payload.(memprotocol.DataReadyRsp)
		readRsp := agent.received[1]
		Expect(ok).To(BeTrue())
		Expect(readRsp.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

func Example() {
	memComp, agent, conn, freq := setupExampleSystem()
	topPort := memComp.Ports.Top
	srcRemote := agent.port.AsRemote()
	dstRemote := topPort.AsRemote()

	startCycles := make(map[uint64]int)
	var pendingReq messaging.Msg
	hasPending := false
	requestsSent := 0
	cycles := 0
	processed := 0
	var latencySum float64

	for agent.completed < numRequests {
		if !hasPending && requestsSent < numRequests {
			pendingReq = makeReadReq(memComp, srcRemote, dstRemote, requestsSent)
			hasPending = true
		}

		if hasPending && agent.port.CanSend() {
			agent.port.Send(pendingReq)
			startCycles[pendingReq.ID] = cycles
			requestsSent++
			hasPending = false
			conn.transfer()
		}

		modelingtest.Tick(memComp)
		conn.transfer()

		latencySum += collectLatency(agent, startCycles, cycles, &processed)
		cycles++
	}

	avgLatencyCycles := latencySum / float64(numRequests)
	totalBytes := uint64(numRequests * readSize)
	seconds := float64(cycles) / float64(freq)
	bandwidthGBS := (float64(totalBytes) / seconds) / 1e9

	fmt.Printf("Achieved bandwidth: %.2f GB/s\n", bandwidthGBS)
	fmt.Printf("Average latency: %.2f cycles\n", avgLatencyCycles)
	// Output:
	// Achieved bandwidth: 64.00 GB/s
	// Average latency: 7.00 cycles
}
