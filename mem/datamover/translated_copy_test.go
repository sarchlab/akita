package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func newBufferedPort(sim *testsim.Simulation) messaging.Port {
	return twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build()
}

// TestCopyThroughAddressTranslator copies to and from a virtual inside range
// that spans two non-contiguous pages. The inside side goes through an
// address translator; its 64-byte units divide the 4 KiB page, so no unit
// crosses a page.
func TestCopyThroughAddressTranslator(t *testing.T) {
	engine := timing.NewSerialEngine()
	sim := testsim.New(engine, false)
	driver := newDriverPort(sim, "Driver.Top", 4)
	conn := direct.NewConnection("Conn", sim, timing.GHz)

	memSpec := idealmemcontroller.DefaultSpec()
	storage := mem.MakeStorageBuilder().WithSimulation(sim).WithCapacity(mem.MB)
	insideStorage := storage.Build("InsideStorage")
	insideMem := buildIdealMem(sim, memSpec, insideStorage, "InsideMem")
	outsideStorage := storage.Build("OutsideStorage")
	outsideMem := buildIdealMem(sim, memSpec, outsideStorage, "OutsideMem")

	const pid = vm.PID(7)
	pageTable := vm.NewPageTable(12)
	pageTable.Insert(vm.Page{PID: pid, VAddr: 0x10000, PAddr: 0x5000, PageSize: 4096, Valid: true})
	pageTable.Insert(vm.Page{PID: pid, VAddr: 0x11000, PAddr: 0x2000, PageSize: 4096, Valid: true})
	iommu := modeling.MakeBuilder[*mmu.Comp](mmu.DefaultSpec()).
		WithSimulation(sim).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("MMU")
	iommu.BindPort("Top", newBufferedPort(sim))
	iommu.BindPort("Control", newBufferedPort(sim))

	atSpec := addresstranslator.DefaultSpec()
	atSpec.NoMigrate = true
	atSpec.Translation.Destinations = []messaging.RemotePort{iommu.Ports.Top.AsRemote()}
	atSpec.Bottom.Destinations = []messaging.RemotePort{insideMem.Ports.Top.AsRemote()}
	at := modeling.MakeBuilder[*addresstranslator.Comp](atSpec).
		WithSimulation(sim).
		Build("AT")
	for _, slot := range []string{"Top", "Bottom", "Translation", "Control"} {
		at.BindPort(slot, newBufferedPort(sim))
	}

	dmSpec := DefaultSpec()
	dmSpec.BufferSize = 1024
	dmSpec.InsideByteGranularity = 64
	dmSpec.OutsideByteGranularity = 256
	dmSpec.Inside.Destinations = []messaging.RemotePort{at.Ports.Top.AsRemote()}
	dmSpec.Outside.Destinations = []messaging.RemotePort{outsideMem.Ports.Top.AsRemote()}
	dm := modeling.MakeBuilder[*Comp](dmSpec).WithSimulation(sim).Build("DataMover")
	ports := makePorts(sim, "DataMover", 4, 16, 16, 4)
	dm.BindPort("Top", ports.Top)
	dm.BindPort("Inside", ports.Inside)
	dm.BindPort("Outside", ports.Outside)
	dm.BindPort("Control", ports.Control)

	for _, p := range []messaging.Port{
		driver, dm.Ports.Top, dm.Ports.Inside, dm.Ports.Outside,
		at.Ports.Top, at.Ports.Bottom, at.Ports.Translation,
		iommu.Ports.Top, insideMem.Ports.Top, outsideMem.Ports.Top,
	} {
		conn.BindPort(p)
	}
	require.NoError(t, sim.Initialize())

	copyRange := func(req datamoverprotocol.Req) {
		t.Helper()
		id := sim.NewID()
		dm.Ports.Top.Deliver(messaging.Msg{ID: id, Src: driver.AsRemote(),
			Dst: dm.Ports.Top.AsRemote(), Payload: req})
		require.NoError(t, engine.Run())
		rsp, ok := driver.RetrieveIncoming()
		require.True(t, ok)
		require.Equal(t, id, rsp.RspTo)
	}

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	outsideStorage.Write(0, data)

	// Physical outside to virtual inside: 0x10800-0x117ff lands in two frames.
	copyRange(datamoverprotocol.Req{
		SrcAddress: 0, DstAddress: 0x10800, ByteSize: 4096,
		SrcSide: datamoverprotocol.SideOutside, DstSide: datamoverprotocol.SideInside,
		DstPID: pid,
	})
	require.Equal(t, data[:2048], insideStorage.Read(0x5800, 2048))
	require.Equal(t, data[2048:], insideStorage.Read(0x2000, 2048))

	// Virtual inside back to physical outside.
	copyRange(datamoverprotocol.Req{
		SrcAddress: 0x10800, DstAddress: 0x8000, ByteSize: 4096,
		SrcSide: datamoverprotocol.SideInside, DstSide: datamoverprotocol.SideOutside,
		SrcPID: pid,
	})
	require.Equal(t, data, outsideStorage.Read(0x8000, 4096))
}
