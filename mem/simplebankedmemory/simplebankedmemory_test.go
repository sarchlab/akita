package simplebankedmemory

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"

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

	port      sim.Port
	completed int
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

		if _, ok := msg.(sim.Rsp); ok {
			a.completed++
		}
	}
}

func (a *bandwidthAgent) NotifyPortFree(sim.Port) {}

func (a *bandwidthAgent) Handle(sim.Event) error {
	return nil
}

type zeroConverter struct{}

func (zeroConverter) ConvertExternalToInternal(uint64) uint64   { return 0 }
func (zeroConverter) ConvertInternalToExternal(v uint64) uint64 { return v }

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
			Build("Mem")

		agent = newTestAgent("Agent")
		conn = newLoopbackConnection("Conn")
		conn.PlugIn(memComp.topPort)
		conn.PlugIn(agent.port)
	})

	AfterEach(func() {
		agent.received = nil
	})

	It("should return read data after configured latency", func() {
		data := []byte{1, 2, 3, 4}
		err := memComp.Storage.Write(0x0, data)
		Expect(err).NotTo(HaveOccurred())

		read := mem.ReadReqBuilder{}.
			WithSrc(agent.port.AsRemote()).
			WithDst(memComp.topPort.AsRemote()).
			WithAddress(0x0).
			WithByteSize(uint64(len(data))).
			Build()

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
		err := memComp.Storage.Write(addr, initial)
		Expect(err).NotTo(HaveOccurred())

		newData := []byte{0x10, 0x20, 0x30, 0x40}

		write := mem.WriteReqBuilder{}.
			WithSrc(agent.port.AsRemote()).
			WithDst(memComp.topPort.AsRemote()).
			WithAddress(addr).
			WithData(newData).
			Build()

		read := mem.ReadReqBuilder{}.
			WithSrc(agent.port.AsRemote()).
			WithDst(memComp.topPort.AsRemote()).
			WithAddress(addr).
			WithByteSize(uint64(len(newData))).
			Build()

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

		committed, err := memComp.Storage.Read(addr, uint64(len(newData)))
		Expect(err).NotTo(HaveOccurred())
		Expect(committed).To(Equal(newData))
	})

	It("should use converted address for bank selection", func() {
		memComp = MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithNumBanks(2).
			WithStageLatency(2).
			WithTopPortBufferSize(4).
			WithAddressConverter(zeroConverter{}).
			Build("MemConv")

		agent = newTestAgent("AgentConv")
		conn = newLoopbackConnection("ConnConv")
		conn.PlugIn(memComp.topPort)
		conn.PlugIn(agent.port)

		write := mem.WriteReqBuilder{}.
			WithSrc(agent.port.AsRemote()).
			WithDst(memComp.topPort.AsRemote()).
			WithAddress(0x0).
			WithData([]byte{1, 2, 3, 4}).
			Build()

		read := mem.ReadReqBuilder{}.
			WithSrc(agent.port.AsRemote()).
			WithDst(memComp.topPort.AsRemote()).
			WithAddress(0x100). // Maps to same internal address as write
			WithByteSize(4).
			Build()

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
	const (
		numRequests = 100000
		readSize    = 64
	)

	engine := sim.NewSerialEngine()
	freq := 1 * sim.GHz

	memComp := MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		WithNumBanks(4).
		WithStageLatency(2).
		WithTopPortBufferSize(32).
		WithPostPipelineBufferSize(32).
		Build("Mem")

	agent := newBandwidthAgent("Agent")
	conn := newLoopbackConnection("Conn")
	conn.PlugIn(memComp.topPort)
	conn.PlugIn(agent.port)

	srcRemote := agent.port.AsRemote()
	dstRemote := memComp.topPort.AsRemote()

	requestsSent := 0
	var pendingReq *mem.ReadReq
	cycles := 0

	for agent.completed < numRequests {
		if pendingReq == nil && requestsSent < numRequests {
			addr := uint64(requestsSent * readSize)
			pendingReq = mem.ReadReqBuilder{}.
				WithSrc(srcRemote).
				WithDst(dstRemote).
				WithAddress(addr).
				WithByteSize(readSize).
				Build()
		}

		if pendingReq != nil {
			if err := agent.port.Send(pendingReq); err == nil {
				requestsSent++
				pendingReq = nil
				conn.transfer()
			}
		}

		memComp.Tick()
		conn.transfer()
		cycles++
	}

	totalBytes := uint64(numRequests * readSize)
	seconds := float64(cycles) / float64(freq)
	bandwidthGBS := (float64(totalBytes) / seconds) / 1e9

	fmt.Printf("Achieved bandwidth: %.2f GB/s\n", bandwidthGBS)
	// Output: Achieved bandwidth: 64.00 GB/s
}
