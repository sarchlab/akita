package multiportsimplemem

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

type stubComponent struct {
	*sim.ComponentBase
}

func newStubComponent(name string) *stubComponent {
	return &stubComponent{
		ComponentBase: sim.NewComponentBase(name),
	}
}

func (c *stubComponent) Handle(sim.Event) error {
	return nil
}

func (c *stubComponent) NotifyRecv(sim.Port) {}

func (c *stubComponent) NotifyPortFree(sim.Port) {}

func TestReadSeesCommittedWriteWhenCompletingSameCycle(t *testing.T) {
	env := newTestEnv(t)
	env.fillWithPendingWrites()

	writeData := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	readReqID := env.enqueueWriteThenRead(writeData)

	env.advanceUntilSlotsFree()
	env.requireNotYetCommitted(writeData)

	env.advanceCycles(env.mem.Latency)
	env.drainConnection()

	env.requireCommitted(writeData)
	env.requireReadReturned(writeData, readReqID)
}

type testEnv struct {
	*require.Assertions
	engine      sim.Engine
	mem         *Comp
	conn        *directconnection.Comp
	agentPorts  []sim.Port
	writeReq    *mem.WriteReq
	readReq     *mem.ReadReq
	pendingData []byte
}

func newTestEnv(t *testing.T) *testEnv {
    t.Helper()
    engine := sim.NewSerialEngine()

	memComp := MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithLatency(4).
		WithConcurrentSlots(2).
		WithNumPorts(2).
		WithPortBufferSize(8).
		WithNewStorage(4 * mem.KB).
		Build("MultiMem")

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	agentPorts := make([]sim.Port, 2)
	for i := 0; i < 2; i++ {
		comp := newStubComponent(fmt.Sprintf("Agent%d", i))
		port := sim.NewPort(comp, 8, 8, fmt.Sprintf("AgentPort%d", i))
		agentPorts[i] = port
		conn.PlugIn(port)
		conn.PlugIn(memComp.Port(i))
	}

	return &testEnv{
		Assertions: require.New(t),
		engine:     engine,
		mem:        memComp,
		conn:       conn,
		agentPorts: agentPorts,
	}
}

func (e *testEnv) fillWithPendingWrites() {
	filler := func(port sim.Port, dst sim.RemotePort, addr uint64, data []byte) {
		req := mem.WriteReqBuilder{}.
			WithSrc(port.AsRemote()).
			WithDst(dst).
			WithAddress(addr).
			WithData(data).
			Build()
		e.send(port, req)
	}

	filler(e.agentPorts[0], e.mem.Port(0).AsRemote(), 0x100, []byte{1, 2, 3, 4})
	filler(e.agentPorts[1], e.mem.Port(1).AsRemote(), 0x200, []byte{5, 6, 7, 8})

	e.True(e.mem.Tick())
	e.drainConnection()

	e.Len(e.mem.activeRequests, 2)
	e.Len(e.mem.waitingRequests, 0)
}

func (e *testEnv) enqueueWriteThenRead(writeData []byte) string {
	write := mem.WriteReqBuilder{}.
		WithSrc(e.agentPorts[0].AsRemote()).
		WithDst(e.mem.Port(0).AsRemote()).
		WithAddress(0x0).
		WithData(writeData).
		Build()
	read := mem.ReadReqBuilder{}.
		WithSrc(e.agentPorts[1].AsRemote()).
		WithDst(e.mem.Port(1).AsRemote()).
		WithAddress(0x0).
		WithByteSize(uint64(len(writeData))).
		Build()

	e.send(e.agentPorts[0], write)
	e.send(e.agentPorts[1], read)

	e.True(e.mem.Tick())
	e.drainConnection()

	e.Len(e.mem.activeRequests, 2)
	e.Len(e.mem.waitingRequests, 2)

	e.readReq = read
	e.writeReq = write
	e.pendingData = writeData

	return read.ID
}

func (e *testEnv) send(port sim.Port, msg sim.Msg) {
    e.Nil(port.Send(msg))
    e.drainConnection()
}

func (e *testEnv) advanceUntilSlotsFree() {
	for i := 0; i < e.mem.Latency-1; i++ {
		e.mem.Tick()
		e.drainConnection()
	}

	e.Len(e.mem.waitingRequests, 0)
	e.Len(e.mem.activeRequests, 2)
}

func (e *testEnv) requireNotYetCommitted(writeData []byte) {
	dataBefore, err := e.mem.Storage.Read(0x0, uint64(len(writeData)))
	e.NoError(err)
	e.NotEqual(writeData, dataBefore)
}

func (e *testEnv) advanceCycles(cycles int) {
	for i := 0; i < cycles; i++ {
		e.mem.Tick()
		e.drainConnection()
	}
}

func (e *testEnv) drainConnection() {
	for e.conn.Tick() {
	}
}

func (e *testEnv) requireCommitted(writeData []byte) {
	dataAfter, err := e.mem.Storage.Read(0x0, uint64(len(writeData)))
	e.NoError(err)
	e.Equal(writeData, dataAfter)
}

func (e *testEnv) requireReadReturned(expected []byte, readReqID string) {
	var readRsp *mem.DataReadyRsp
	for {
		item := e.agentPorts[1].RetrieveIncoming()
		if item == nil {
			break
		}

		rsp, ok := item.(*mem.DataReadyRsp)
		if !ok {
			continue
		}

		if rsp.GetRspTo() == readReqID {
			readRsp = rsp
		}
	}

	e.NotNil(readRsp, "read response not received")
	e.Equal(expected, readRsp.Data)
}
