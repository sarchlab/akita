package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// buildFaultingMMU builds an MMU with the given fault handler and a Top port
// of topBufSize.
func buildFaultingMMU(
	t *testing.T,
	handler messaging.RemotePort,
	topBufSize int,
) (*Comp, *vm.PageTable) {
	t.Helper()

	sim := testsim.New(timing.NewSerialEngine(), false)
	pageTable := vm.NewPageTable(12)

	spec := DefaultSpec()
	spec.FaultHandler = handler

	mmuComp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable}).
		Build("MMU")
	ports := makePorts(sim, "MMU", topBufSize)
	mmuComp.BindPort("Top", ports.Top)
	mmuComp.BindPort("Control", ports.Control)
	(&noopConn{}).BindPort(mmuComp.Ports.Top)
	(&noopConn{}).BindPort(mmuComp.Ports.Control)
	require.NoError(t, sim.Initialize())

	return mmuComp, pageTable
}

// walk finishes a walk of page, requested by device 1.
func walk(t *testing.T, mmuComp *Comp, pageTable *vm.PageTable, page vm.Page) {
	t.Helper()

	pageTable.Insert(page)
	mmuComp.State.WalkingTranslations = []transactionState{{
		ReqID:    1,
		ReqSrc:   "L2TLB.Bottom",
		PID:      uint32(page.PID),
		VAddr:    page.VAddr,
		DeviceID: 1,
	}}

	require.True(t, mmuComp.Middlewares.Translation.Handle(nil))
}

func remoteUnifiedPage() vm.Page {
	return vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000, DeviceID: 2,
		Valid: true, Unified: true}
}

func outgoing(port messaging.Port) []messaging.Msg {
	var msgs []messaging.Msg

	for {
		msg, ok := port.RetrieveOutgoing()
		if !ok {
			return msgs
		}

		msgs = append(msgs, msg)
	}
}

func TestMMUFaultsOnARemoteUnifiedPage(t *testing.T) {
	mmuComp, pageTable := buildFaultingMMU(t, "Driver.PageMigration", 16)
	walk(t, mmuComp, pageTable, remoteUnifiedPage())

	msgs := outgoing(mmuComp.Ports.Top)
	require.Len(t, msgs, 1)
	rsp := msgs[0].Payload.(addrtransprotocol.Rsp)
	require.True(t, rsp.Fault)
	require.Equal(t, messaging.RemotePort("L2TLB.Bottom"), msgs[0].Dst)
	require.Empty(t, mmuComp.State.WalkingTranslations)

	// The notification follows the response, from the same port.
	require.True(t, mmuComp.Middlewares.Translation.Handle(nil))
	msgs = outgoing(mmuComp.Ports.Top)
	require.Len(t, msgs, 1)
	fault := msgs[0].Payload.(addrtransprotocol.PageFault)
	require.Equal(t, messaging.RemotePort("Driver.PageMigration"), msgs[0].Dst)
	require.Equal(t, uint64(0x1000), fault.VAddr)
	require.Equal(t, uint64(1), fault.DeviceID)
	require.Equal(t, uint64(2), fault.Page.DeviceID)
	require.Equal(t, uint64(1), mmuComp.State.NumFaults)
}

func TestMMUDoesNotFaultWithoutAHandler(t *testing.T) {
	mmuComp, pageTable := buildFaultingMMU(t, "", 16)
	walk(t, mmuComp, pageTable, remoteUnifiedPage())

	msgs := outgoing(mmuComp.Ports.Top)
	require.Len(t, msgs, 1)
	require.False(t, msgs[0].Payload.(addrtransprotocol.Rsp).Fault)
	require.Zero(t, mmuComp.State.NumFaults)
}

func TestMMUDoesNotFaultOnPinnedLocalOrNonUnifiedPages(t *testing.T) {
	pinned := remoteUnifiedPage()
	pinned.IsPinned = true

	local := remoteUnifiedPage()
	local.DeviceID = 1

	notUnified := remoteUnifiedPage()
	notUnified.Unified = false

	for name, page := range map[string]vm.Page{
		"pinned": pinned, "local": local, "not unified": notUnified,
	} {
		t.Run(name, func(t *testing.T) {
			mmuComp, pageTable := buildFaultingMMU(t, "Driver.PageMigration", 16)
			walk(t, mmuComp, pageTable, page)

			msgs := outgoing(mmuComp.Ports.Top)
			require.Len(t, msgs, 1)
			require.False(t, msgs[0].Payload.(addrtransprotocol.Rsp).Fault)
			require.Empty(t, mmuComp.State.PendingFaults)
		})
	}
}
