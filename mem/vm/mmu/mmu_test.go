package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the MMU now owns its ports (they are no longer
// injectable), tests feed requests with Deliver and read responses with
// RetrieveOutgoing; the port still needs a connection so its send/retrieve
// notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(_ messaging.Port)      {}

// makePorts creates the ports of the MMU named name, with the given Top
// buffer size and a Control buffer size of 4.
func makePorts(name string, topBufSize int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(topBufSize, topBufSize),
		Control: twowaybuffered.NewPort(4, 4),
	}
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestMMU(t *testing.T) {
	var (
		engine           timing.Engine
		sim              timing.Simulation
		pageTable        *vm.PageTable
		mmuComp          *Comp
		topPort          messaging.Port
		translationMWRef *translationMW
	)

	// build constructs an MMU with the given Top buffer size, injects the
	// shared page table, and plugs noopConns so its ports can be driven.
	build := func(topBufSize int) {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		mmuComp = modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("MMU")
		mmuCompPorts := makePorts("MMU", topBufSize)
		mmuComp.BindPort("Top", mmuCompPorts.Top)
		mmuComp.BindPort("Control", mmuCompPorts.Control)

		topPort = mmuComp.Ports.Top

		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(mmuComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		translationMWRef = mmuComp.Middlewares.Translation
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		pageTable = vm.NewPageTable(12)
		build(4096)
	}

	t.Run("parse top", func(t *testing.T) {
		t.Run("should process translation request", func(t *testing.T) {
			setup1(t)

			translationReq := messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100000100,
				DeviceID: 0},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent.Top"),
				Dst: topPort.AsRemote(),

				TrafficClass: "vmprotocol.TranslationReq"}

			topPort.Deliver(translationReq)

			translationMWRef.parseFromTop()

			next := &mmuComp.State
			require.Len(t, next.WalkingTranslations, 1)
		})

		t.Run("should stall parse from top "+
			"if MMU is servicing max requests", func(t *testing.T) {
			setup1(t)

			mmuComp.State = state{
				WalkingTranslations: make([]transactionState, 16),
			}

			madeProgress := translationMWRef.parseFromTop()

			require.False(t, madeProgress)
		})
	})

	t.Run("walk page table", func(t *testing.T) {
		t.Run("should reduce translation cycles", func(t *testing.T) {
			setup1(t)

			mmuComp.State = state{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.NewID(),
						ReqDst:    topPort.AsRemote(),
						PID:       1,
						VAddr:     0x1020,
						DeviceID:  0,
						CycleLeft: 10,
					},
				},
			}

			madeProgress := translationMWRef.walkPageTable()

			next := &mmuComp.State
			require.Equal(t, 9, next.WalkingTranslations[0].CycleLeft)
			require.True(t, madeProgress)
		})

		t.Run("should send rsp to top if hit", func(t *testing.T) {
			setup1(t)

			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}
			pageTable.Insert(page)

			mmuComp.State = state{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.NewID(),
						ReqSrc:    messaging.RemotePort("Agent.Top"),
						ReqDst:    topPort.AsRemote(),
						PID:       1,
						VAddr:     0x1000,
						DeviceID:  0,
						CycleLeft: 0,
					},
				},
			}

			madeProgress := translationMWRef.walkPageTable()

			require.True(t, madeProgress)
			next := &mmuComp.State
			require.Len(t, next.WalkingTranslations, 0)

			rsp, _ := topPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: vmprotocol.TranslationRsp{}}, rsp)
			require.Equal(t, page, rsp.Payload.(vmprotocol.TranslationRsp).Page)
		})

		t.Run("should stall if cannot send to top", func(t *testing.T) {
			setup1(t)

			// Rebuild with a single-slot Top port and pre-fill its outgoing
			// buffer so the controller's response Send fails.
			build(1)

			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}
			pageTable.Insert(page)

			dummy := messaging.Msg{ID: sim.NewID(), Payload: vmprotocol.TranslationRsp{},
				Src:          topPort.AsRemote(),
				Dst:          messaging.RemotePort("Agent.Top"),
				TrafficClass: "vmprotocol.TranslationRsp"}

			topPort.Send(dummy)

			mmuComp.State = state{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.NewID(),
						ReqSrc:    messaging.RemotePort("Agent.Top"),
						ReqDst:    topPort.AsRemote(),
						PID:       1,
						VAddr:     0x1000,
						DeviceID:  0,
						CycleLeft: 0,
					},
				},
			}

			madeProgress := translationMWRef.walkPageTable()

			require.False(t, madeProgress)
		})
	})
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestMMUIntegration(t *testing.T) {
	var (
		engine    timing.Engine
		sim       timing.Simulation
		mmuComp   *Comp
		pageTable *vm.PageTable
		topPort   messaging.Port
		agentPort messaging.Port
	)

	setup2 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		pageTable = vm.NewPageTable(12)

		mmuComp = modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("MMU")
		mmuCompPorts := makePorts("MMU", 4096)
		mmuComp.BindPort("Top", mmuCompPorts.Top)
		mmuComp.BindPort("Control", mmuCompPorts.Control)

		topPort = mmuComp.Ports.Top
		(&noopConn{}).BindPort(topPort)

		agentPort = twowaybuffered.NewPort(4, 4)
		agentPort.BindOwner(testDriver{}, "Agent")
		(&noopConn{}).BindPort(agentPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	t.Run("should lookup", func(t *testing.T) {
		setup2(t)

		page := vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 1,
		}
		pageTable.Insert(page)

		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: agentPort.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		topPort.Deliver(req)

		// Drive enough ticks for the request to be parsed, walked, and
		// answered (default latency is 10).
		for i := 0; i < 20; i++ {
			modelingtest.Tick(mmuComp)
		}

		rspI, _ := topPort.RetrieveOutgoing()
		require.NotNil(t, rspI)
		rsp := rspI
		require.Equal(t, page, rsp.Payload.(vmprotocol.TranslationRsp).Page)
		require.Equal(t, req.ID, rsp.RspTo)
	})
}

type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
