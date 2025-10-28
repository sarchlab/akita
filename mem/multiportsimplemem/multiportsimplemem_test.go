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
	require := require.New(t)

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

	send := func(port sim.Port, msg sim.Msg) {
		require.Nil(port.Send(msg))
		for conn.Tick() {
		}
	}

	// Occupy both slots with filler writes so that later requests must wait.
	filler0 := mem.WriteReqBuilder{}.
		WithSrc(agentPorts[0].AsRemote()).
		WithDst(memComp.Port(0).AsRemote()).
		WithAddress(0x100).
		WithData([]byte{1, 2, 3, 4}).
		Build()
	filler1 := mem.WriteReqBuilder{}.
		WithSrc(agentPorts[1].AsRemote()).
		WithDst(memComp.Port(1).AsRemote()).
		WithAddress(0x200).
		WithData([]byte{5, 6, 7, 8}).
		Build()
	send(agentPorts[0], filler0)
	send(agentPorts[1], filler1)

	require.True(memComp.Tick())
	for conn.Tick() {
	}

	require.Len(memComp.activeRequests, 2)
	require.Len(memComp.waitingRequests, 0)

	// Enqueue the write (older) and read (newer) requests that target the same address.
	writeData := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	writeReq := mem.WriteReqBuilder{}.
		WithSrc(agentPorts[0].AsRemote()).
		WithDst(memComp.Port(0).AsRemote()).
		WithAddress(0x0).
		WithData(writeData).
		Build()
	readReq := mem.ReadReqBuilder{}.
		WithSrc(agentPorts[1].AsRemote()).
		WithDst(memComp.Port(1).AsRemote()).
		WithAddress(0x0).
		WithByteSize(uint64(len(writeData))).
		Build()

	send(agentPorts[0], writeReq)
	send(agentPorts[1], readReq)

	require.True(memComp.Tick())
	for conn.Tick() {
	}

	require.Len(memComp.activeRequests, 2)
	require.Len(memComp.waitingRequests, 2)

	// Advance cycles so the filler requests finish and free both slots together.
	for i := 0; i < memComp.Latency-1; i++ {
		memComp.Tick()
		for conn.Tick() {
		}
	}

	require.Len(memComp.waitingRequests, 0)
	require.Len(memComp.activeRequests, 2)

	// Ensure the write has not committed yet.
	dataBefore, err := memComp.Storage.Read(0x0, uint64(len(writeData)))
	require.NoError(err)
	require.NotEqual(writeData, dataBefore)

	// Let the queued write and read finish together.
	for i := 0; i < memComp.Latency; i++ {
		memComp.Tick()
		for conn.Tick() {
		}
	}

	// Drain any remaining connection activity.
	for conn.Tick() {
	}

	// Verify the write is now visible in storage.
	dataAfter, err := memComp.Storage.Read(0x0, uint64(len(writeData)))
	require.NoError(err)
	require.Equal(writeData, dataAfter)

	var readRsp *mem.DataReadyRsp
	for {
		item := agentPorts[1].RetrieveIncoming()
		if item == nil {
			break
		}

		switch rsp := item.(type) {
		case *mem.DataReadyRsp:
			if rsp.GetRspTo() == readReq.ID {
				readRsp = rsp
			}
		}
	}

	require.NotNil(readRsp, "read response not received")
	require.Equal(writeData, readRsp.Data)
}
