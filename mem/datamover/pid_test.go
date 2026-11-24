package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

// TestCopyCarriesSidePIDs checks that every read of a copy carries SrcPID and
// every write carries DstPID.
func TestCopyCarriesSidePIDs(t *testing.T) {
	simulation := akitasim.MakeBuilder().Build()
	spec := DefaultSpec()
	spec.Inside.Destinations = []messaging.RemotePort{"Inside.Top"}
	spec.Outside.Destinations = []messaging.RemotePort{"Outside.Top"}
	spec.InsideByteGranularity = 64
	spec.OutsideByteGranularity = 64
	spec.BufferSize = 128
	c := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(simulation).
		Build("DataMover")
	ports := makePorts(simulation, "DataMover", 1, 1, 1, 1)
	c.BindPort("Top", ports.Top)
	c.BindPort("Inside", ports.Inside)
	c.BindPort("Outside", ports.Outside)
	c.BindPort("Control", ports.Control)
	for _, p := range []messaging.Port{ports.Top, ports.Inside, ports.Outside, ports.Control} {
		(&ccNoopConn{}).BindPort(p)
	}
	require.NoError(t, simulation.Initialize())

	c.Ports.Top.Deliver(messaging.Msg{ID: c.NewID(), Payload: datamoverprotocol.Req{
		SrcAddress: 0x1000,
		DstAddress: 0x2000,
		ByteSize:   128,
		SrcSide:    datamoverprotocol.SideOutside,
		DstSide:    datamoverprotocol.SideInside,
		SrcPID:     vm.PID(0),
		DstPID:     vm.PID(7),
	}})
	require.True(t, c.Middlewares.CtrlParse.parseFromCP())

	dt := c.Middlewares.DataTransfer
	for range 2 {
		require.True(t, dt.readFromSrc())
		read, ok := c.Ports.Outside.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, vm.PID(0), read.Payload.(memprotocol.ReadReq).PID)

		c.Ports.Outside.Deliver(messaging.Msg{ID: c.NewID(), RspTo: read.ID,
			Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
		require.True(t, dt.processDataReady(c.Ports.Outside))

		require.True(t, dt.writeToDst())
		write, ok := c.Ports.Inside.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, vm.PID(7), write.Payload.(memprotocol.WriteReq).PID)
	}

	// The opposite direction swaps which PID each request carries.
	c.State.Transactions = nil
	c.Ports.Top.Deliver(messaging.Msg{ID: c.NewID(), Payload: datamoverprotocol.Req{
		SrcAddress: 0x2000,
		DstAddress: 0x1000,
		ByteSize:   64,
		SrcSide:    datamoverprotocol.SideInside,
		DstSide:    datamoverprotocol.SideOutside,
		SrcPID:     vm.PID(7),
	}})
	require.True(t, c.Middlewares.CtrlParse.parseFromCP())
	require.True(t, dt.readFromSrc())
	read, ok := c.Ports.Inside.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, vm.PID(7), read.Payload.(memprotocol.ReadReq).PID)
	c.Ports.Inside.Deliver(messaging.Msg{ID: c.NewID(), RspTo: read.ID,
		Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
	require.True(t, dt.processDataReady(c.Ports.Inside))
	require.True(t, dt.writeToDst())
	write, ok := c.Ports.Outside.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, vm.PID(0), write.Payload.(memprotocol.WriteReq).PID)
}
