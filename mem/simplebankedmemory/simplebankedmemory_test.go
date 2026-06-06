package simplebankedmemory

import (
	"fmt"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// assignPort builds a port instance for a declared component port and attaches
// it, using the same registrar the component builder used.
func assignPort(
	reg modeling.Registrar,
	comp *Comp,
	name string,
	bufSize int,
) {
	p := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, p)
}

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
		msg := src.PeekOutgoing()
		if msg == nil {
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
	*messaging.PortOwnerBase

	name     string
	port     messaging.Port
	received []messaging.Msg
}

func newTestAgent(name string) *testAgent {
	naming.MustBeValid(name)

	a := &testAgent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}

	a.port = messaging.NewPort(a, 4, 4, fmt.Sprintf("%s.Port", name))
	a.AddPort("Port", a.port)

	return a
}

func (a *testAgent) Name() string {
	return a.name
}

func (a *testAgent) NotifyRecv(port messaging.Port) {
	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			break
		}

		a.received = append(a.received, msg)
	}
}

func (a *testAgent) NotifyPortFree(messaging.Port) {
	// No-op.
}

func (a *testAgent) Handle(timing.Event) error {
	return nil
}

func (a *testAgent) send(msg messaging.Msg) {
	Expect(a.port.CanSend()).To(BeTrue())
	a.port.Send(msg)
}

type bandwidthAgent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	name         string
	port         messaging.Port
	completed    int
	completedIDs []uint64
}

func newBandwidthAgent(name string) *bandwidthAgent {
	naming.MustBeValid(name)

	a := &bandwidthAgent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}

	a.port = messaging.NewPort(a, 8, 8, fmt.Sprintf("%s.Port", name))
	a.AddPort("Port", a.port)

	return a
}

func (a *bandwidthAgent) Name() string {
	return a.name
}

func (a *bandwidthAgent) NotifyRecv(port messaging.Port) {
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

func (a *bandwidthAgent) NotifyPortFree(messaging.Port) {}

func (a *bandwidthAgent) Handle(timing.Event) error {
	return nil
}

const (
	numRequests = 100000
	readSize    = 64
)

func setupExampleSystem() (*Comp, *bandwidthAgent, *loopbackConnection, timing.Freq) {
	engine := timing.NewSerialEngine()
	freq := 1 * timing.GHz

	spec := DefaultSpec()
	spec.Freq = freq
	spec.NumBanks = 16
	spec.StageLatency = 6
	spec.PostPipelineBufSize = 32

	reg := modeling.NewStandaloneRegistrar(engine)
	memComp := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		Build("Mem")

	assignPort(reg, memComp, "Top", 32)
	assignPort(reg, memComp, "Control", 16)

	topPort := memComp.GetPortByName("Top")
	agent := newBandwidthAgent("Agent")
	conn := newLoopbackConnection("Conn")
	conn.PlugIn(topPort)
	conn.PlugIn(agent.port)

	return memComp, agent, conn, freq
}

func makeReadReq(src, dst messaging.RemotePort, index int) mem.ReadReq {
	addr := uint64(index * readSize)
	r := mem.ReadReq{}
	r.ID = timing.GetIDGenerator().Generate()
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
		memComp *Comp
		storage *mem.Storage
		agent   *testAgent
		conn    *loopbackConnection
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 2

		reg := modeling.NewStandaloneRegistrar(engine)
		memComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{Storage: storage}).
			Build("Mem")

		assignPort(reg, memComp, "Top", 4)
		assignPort(reg, memComp, "Control", 16)

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
		err := storage.Write(0x0, data)
		Expect(err).NotTo(HaveOccurred())

		topPort := memComp.GetPortByName("Top")
		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
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
		rsp := agent.received[0].(mem.DataReadyRsp)
		Expect(rsp.Data).To(Equal(data))
	})

	It("should commit write before serving subsequent read", func() {
		addr := uint64(0x100)

		initial := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		err := storage.Write(addr, initial)
		Expect(err).NotTo(HaveOccurred())

		newData := []byte{0x10, 0x20, 0x30, 0x40}

		topPort := memComp.GetPortByName("Top")

		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = agent.port.AsRemote()
		write.Dst = topPort.AsRemote()
		write.Address = addr
		write.Data = newData
		write.TrafficBytes = len(newData) + 12
		write.TrafficClass = "mem.WriteReq"

		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
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

		_, isWriteDone := agent.received[0].(mem.WriteDoneRsp)
		Expect(isWriteDone).To(BeTrue())

		readRsp, ok := agent.received[1].(mem.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(readRsp.Data).To(Equal(newData))

		committed, err := storage.Read(addr, uint64(len(newData)))
		Expect(err).NotTo(HaveOccurred())
		Expect(committed).To(Equal(newData))
	})

	It("should use converted address for storage access", func() {
		// Use the interleaving converter: InterleavingSize=256, 2 elements,
		// index 0. External address 0x0 maps to internal address 0x0. External
		// address 0x200 maps to internal address 0x100.
		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 2
		spec.AddrConvKind = "interleaving"
		spec.AddrInterleavingSize = 256
		spec.AddrTotalNumOfElements = 2
		spec.AddrCurrentElementIndex = 0
		spec.AddrOffset = 0

		reg := modeling.NewStandaloneRegistrar(engine)
		memComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("MemConv")

		assignPort(reg, memComp, "Top", 4)
		assignPort(reg, memComp, "Control", 16)

		topPort := memComp.GetPortByName("Top")
		agent = newTestAgent("AgentConv")
		conn = newLoopbackConnection("ConnConv")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)

		// Write 4 bytes at external address 0x0 → internal 0x0.
		convWriteData := []byte{1, 2, 3, 4}
		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = agent.port.AsRemote()
		write.Dst = topPort.AsRemote()
		write.Address = 0x0
		write.Data = convWriteData
		write.TrafficBytes = len(convWriteData) + 12
		write.TrafficClass = "mem.WriteReq"

		// Read 4 bytes at external address 0x0 → internal 0x0.
		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
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

		readRsp, ok := agent.received[1].(mem.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(readRsp.Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

func Example() {
	memComp, agent, conn, freq := setupExampleSystem()
	topPort := memComp.GetPortByName("Top")
	srcRemote := agent.port.AsRemote()
	dstRemote := topPort.AsRemote()

	startCycles := make(map[uint64]int)
	var pendingReq mem.ReadReq
	hasPending := false
	requestsSent := 0
	cycles := 0
	processed := 0
	var latencySum float64

	for agent.completed < numRequests {
		if !hasPending && requestsSent < numRequests {
			pendingReq = makeReadReq(srcRemote, dstRemote, requestsSent)
			hasPending = true
		}

		if hasPending && agent.port.CanSend() {
			agent.port.Send(pendingReq)
			startCycles[pendingReq.ID] = cycles
			requestsSent++
			hasPending = false
			conn.transfer()
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
