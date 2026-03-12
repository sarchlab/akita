package simplebankedmemory

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type loopbackConnection struct {
	sim.HookableBase

	name  string
	ports []sim.Port
}

func newLoopbackConnection(name string) *loopbackConnection {
	return &loopbackConnection{
		name: name,
	}
}

func (c *loopbackConnection) Name() string {
	return c.name
}

func (c *loopbackConnection) PlugIn(port sim.Port) {
	c.ports = append(c.ports, port)
	port.SetConnection(c)
}

func (c *loopbackConnection) Unplug(sim.Port) {
	panic("not implemented")
}

func (c *loopbackConnection) NotifyAvailable(sim.Port) {
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

func (c *loopbackConnection) forward(src, dst sim.Port) {
	for {
		msg := src.PeekOutgoing()
		if msg == nil {
			break
		}

		if err := dst.Deliver(msg); err != nil {
			break
		}

		src.RetrieveOutgoing()
	}
}

type testAgent struct {
	*sim.ComponentBase

	port     sim.Port
	received []sim.Msg
}

func newTestAgent(name string) *testAgent {
	a := &testAgent{
		ComponentBase: sim.NewComponentBase(name),
	}

	a.port = sim.NewPort(a, 4, 4, fmt.Sprintf("%s.Port", name))
	a.AddPort("Port", a.port)

	return a
}

func (a *testAgent) NotifyRecv(port sim.Port) {
	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			break
		}

		a.received = append(a.received, msg)
	}
}

func (a *testAgent) NotifyPortFree(sim.Port) {
	// No-op.
}

func (a *testAgent) Handle(sim.Event) error {
	return nil
}

func (a *testAgent) send(msg sim.Msg) {
	sendErr := a.port.Send(msg)
	Expect(sendErr).To(BeNil())
}

type bandwidthAgent struct {
	*sim.ComponentBase

	port         sim.Port
	completed    int
	completedIDs []string
}

func newBandwidthAgent(name string) *bandwidthAgent {
	a := &bandwidthAgent{
		ComponentBase: sim.NewComponentBase(name),
	}

	a.port = sim.NewPort(a, 8, 8, fmt.Sprintf("%s.Port", name))
	a.AddPort("Port", a.port)

	return a
}

func (a *bandwidthAgent) NotifyRecv(port sim.Port) {
	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			break
		}

		if msg.Meta().IsRsp() {
			a.completed++
			a.completedIDs = append(a.completedIDs, msg.Meta().RspTo)
		}
	}
}

func (a *bandwidthAgent) NotifyPortFree(sim.Port) {}

func (a *bandwidthAgent) Handle(sim.Event) error {
	return nil
}

const (
	numRequests = 100000
	readSize    = 64
)

func setupExampleSystem() (*Comp, *bandwidthAgent, *loopbackConnection, sim.Freq) {
	engine := sim.NewSerialEngine()
	freq := 1 * sim.GHz

	memComp := MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		WithNumBanks(16).
		WithStageLatency(6).
		WithTopPortBufferSize(32).
		WithPostPipelineBufferSize(32).
		WithTopPort(sim.NewPort(nil, 32, 32, "Mem.TopPort")).
		Build("Mem")

	topPort := memComp.GetPortByName("Top")
	agent := newBandwidthAgent("Agent")
	conn := newLoopbackConnection("Conn")
	conn.PlugIn(topPort)
	conn.PlugIn(agent.port)

	return memComp, agent, conn, freq
}

func makeReadReq(src, dst sim.RemotePort, index int) *mem.ReadReq {
	addr := uint64(index * readSize)
	r := &mem.ReadReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.Address = addr
	r.AccessByteSize = readSize
	r.TrafficBytes = 12
	r.TrafficClass = "mem.ReadReq"
	return r
}

func collectLatency(
	agent *bandwidthAgent,
	startCycles map[string]int,
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
		engine  sim.Engine
		memComp *Comp
		agent   *testAgent
		conn    *loopbackConnection
	)

	BeforeEach(func() {
		engine = sim.NewSerialEngine()
		memComp = MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithNumBanks(2).
			WithStageLatency(2).
			WithTopPortBufferSize(4).
			WithTopPort(sim.NewPort(nil, 4, 4, "Mem.TopPort")).
			Build("Mem")

		topPort := memComp.GetPortByName("Top")
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
		err := memComp.GetStorage().Write(0x0, data)
		Expect(err).NotTo(HaveOccurred())

		topPort := memComp.GetPortByName("Top")
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agent.port.AsRemote()
		read.Dst = topPort.AsRemote()
		read.Address = 0x0
		read.AccessByteSize = uint64(len(data))
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		agent.send(read)

		for i := 0; i < 6; i++ {
			memComp.Tick()
		}

		Expect(agent.received).To(HaveLen(1))
		rsp := agent.received[0].(*mem.DataReadyRsp)
		Expect(rsp.Data).To(Equal(data))
	})

	It("should commit write before serving subsequent read", func() {
		addr := uint64(0x100)

		initial := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		err := memComp.GetStorage().Write(addr, initial)
		Expect(err).NotTo(HaveOccurred())

		newData := []byte{0x10, 0x20, 0x30, 0x40}

		topPort := memComp.GetPortByName("Top")

		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = agent.port.AsRemote()
		write.Dst = topPort.AsRemote()
		write.Address = addr
		write.Data = newData
		write.TrafficBytes = len(newData) + 12
		write.TrafficClass = "mem.WriteReq"

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agent.port.AsRemote()
		read.Dst = topPort.AsRemote()
		read.Address = addr
		read.AccessByteSize = uint64(len(newData))
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		agent.send(write)
		agent.send(read)

		for i := 0; i < 10; i++ {
			memComp.Tick()
		}

		Expect(agent.received).To(HaveLen(2))

		_, isWriteDone := agent.received[0].(*mem.WriteDoneRsp)
		Expect(isWriteDone).To(BeTrue())

		readRsp, ok := agent.received[1].(*mem.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(readRsp.Data).To(Equal(newData))

		committed, err := memComp.GetStorage().Read(addr, uint64(len(newData)))
		Expect(err).NotTo(HaveOccurred())
		Expect(committed).To(Equal(newData))
	})

	It("should use converted address for storage access", func() {
		// Use InterleavingConverter: InterleavingSize=256, 2 elements, index 0.
		// External address 0x0 maps to internal address 0x0.
		// External address 0x200 maps to internal address 0x100.
		converter := mem.InterleavingConverter{
			InterleavingSize:    256,
			TotalNumOfElements:  2,
			CurrentElementIndex: 0,
			Offset:              0,
		}

		memComp = MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithNumBanks(2).
			WithStageLatency(2).
			WithTopPortBufferSize(4).
			WithAddressConverter(converter).
			WithTopPort(sim.NewPort(nil, 4, 4, "MemConv.TopPort")).
			Build("MemConv")

		topPort := memComp.GetPortByName("Top")
		agent = newTestAgent("AgentConv")
		conn = newLoopbackConnection("ConnConv")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)

		// Write 4 bytes at external address 0x0 → internal 0x0.
		convWriteData := []byte{1, 2, 3, 4}
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = agent.port.AsRemote()
		write.Dst = topPort.AsRemote()
		write.Address = 0x0
		write.Data = convWriteData
		write.TrafficBytes = len(convWriteData) + 12
		write.TrafficClass = "mem.WriteReq"

		// Read 4 bytes at external address 0x0 → internal 0x0.
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agent.port.AsRemote()
		read.Dst = topPort.AsRemote()
		read.Address = 0x0
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		agent.send(write)
		agent.send(read)

		for i := 0; i < 12; i++ {
			memComp.Tick()
		}

		Expect(agent.received).To(HaveLen(2))

		readRsp, ok := agent.received[1].(*mem.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(readRsp.Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

func Example() {
	memComp, agent, conn, freq := setupExampleSystem()
	topPort := memComp.GetPortByName("Top")
	srcRemote := agent.port.AsRemote()
	dstRemote := topPort.AsRemote()

	startCycles := make(map[string]int)
	var pendingReq *mem.ReadReq
	requestsSent := 0
	cycles := 0
	processed := 0
	var latencySum float64

	for agent.completed < numRequests {
		if pendingReq == nil && requestsSent < numRequests {
			pendingReq = makeReadReq(srcRemote, dstRemote, requestsSent)
		}

		if pendingReq != nil {
			if err := agent.port.Send(pendingReq); err == nil {
				startCycles[pendingReq.ID] = cycles
				requestsSent++
				pendingReq = nil
				conn.transfer()
			}
		}

		memComp.Tick()
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
