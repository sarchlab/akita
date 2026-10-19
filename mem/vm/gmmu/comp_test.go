package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the GMMU now owns its ports (they are no longer
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

func TestGMMU(t *testing.T) {
	var (
		engine     timing.Engine
		sim        *testsim.Simulation
		pageTable  *vm.PageTable
		gmmuComp   *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		mw         *walkMW
	)

	const (
		agentPort     = messaging.RemotePort("Agent")
		lowModulePort = messaging.RemotePort("LowModule")
	)

	// build constructs a GMMU, injects the shared page table, and plugs a
	// noopConn into each port so they can be driven.
	build := func() {
		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.Bottom = lowModulePort

		gmmuComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("MMU")
		gmmuCompPorts := defaultPorts(sim, "MMU")
		gmmuComp.BindPort("Top", gmmuCompPorts.Top)
		gmmuComp.BindPort("Bottom", gmmuCompPorts.Bottom)
		gmmuComp.BindPort("Control", gmmuCompPorts.Control)

		topPort = gmmuComp.Ports.Top
		bottomPort = gmmuComp.Ports.Bottom

		topConn := &noopConn{}
		topConn.BindPort(topPort)
		bottomConn := &noopConn{}
		bottomConn.BindPort(bottomPort)
		(&noopConn{}).BindPort(gmmuComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
		mw = gmmuComp.Middlewares.Walk
	}

	makeTranslationReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: agentPort,
			Dst: topPort.AsRemote(),
		}

		return req
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		pageTable = vm.NewPageTable(12)
		build()
	}

	t.Run("GMMU Builder", func(t *testing.T) {
		t.Run("should build GMMU correctly", func(t *testing.T) {
			setup1(t)

			require.Equal(t, 1*timing.GHz, gmmuComp.Spec.Freq)
			require.Equal(t, 16, gmmuComp.Spec.MaxRequestsInFlight)
			require.Equal(t, pageTable, mw.pageTable)
			require.Equal(t, uint64(0), gmmuComp.Spec.DeviceID)
		})
	})

	t.Run("GMMU parse from top", func(t *testing.T) {
		t.Run("should process translation request", func(t *testing.T) {
			setup1(t)

			topPort.Deliver(makeTranslationReq(0x00000000))

			mw.Handle(modelingtest.TickEvent(gmmuComp))

			state := &gmmuComp.State
			require.Len(t, state.WalkingTranslations, 1)
		})

		t.Run("should walk page table", func(t *testing.T) {
			setup1(t)

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000000,
				DeviceID: 0,
				Valid:    true,
			}
			pageTable.Insert(page)

			topPort.Deliver(makeTranslationReq(0x10000000))

			// Tick 1: parseFromTop accepts the request into component state.
			modelingtest.Tick(gmmuComp)
			// Tick 2: walkPageTable sees the translation and decrements
			// CycleLeft (latency=1 → 0).
			modelingtest.Tick(gmmuComp)
			// Tick 3: CycleLeft==0, page walk completes and sends response.
			modelingtest.Tick(gmmuComp)

			rspI, _ := topPort.RetrieveOutgoing()
			require.NotNil(t, rspI)
			rsp := rspI
			require.Equal(t, page, rsp.Payload.(vmprotocol.TranslationRsp).Page)
			require.Equal(t, vm.PID(1), rsp.Payload.(vmprotocol.TranslationRsp).Page.PID)
		})

		t.Run("should send request remotely", func(t *testing.T) {
			setup1(t)

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000000,
				DeviceID: 1,
				Valid:    true,
			}
			pageTable.Insert(page)

			topPort.Deliver(makeTranslationReq(0x10000000))

			// Tick 1: parseFromTop adds translation to state.
			modelingtest.Tick(gmmuComp)
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			modelingtest.Tick(gmmuComp)
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			modelingtest.Tick(gmmuComp)

			reqI, _ := bottomPort.RetrieveOutgoing()
			require.NotNil(t, reqI)
			req := reqI
			require.Equal(t, lowModulePort, req.Dst)
			require.Equal(t, uint64(0x10000000), req.Payload.(vmprotocol.TranslationReq).VAddr)
		})

		t.Run("should return response from remote page table", func(t *testing.T) {
			setup1(t)

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000000,
				DeviceID: 1,
				Valid:    true,
			}
			pageTable.Insert(page)

			topPort.Deliver(makeTranslationReq(0x10000000))

			// Tick 1: parseFromTop adds translation to state.
			modelingtest.Tick(gmmuComp)
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			modelingtest.Tick(gmmuComp)
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			modelingtest.Tick(gmmuComp)

			reqI, _ := bottomPort.RetrieveOutgoing()
			require.NotNil(t, reqI)
			sentReqToBottom := reqI

			// Deliver the response from the bottom (remote page table).
			rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: page,
			},
				ID:    sim.NewID(),
				Src:   lowModulePort,
				Dst:   bottomPort.AsRemote(),
				RspTo: sentReqToBottom.ID,
			}

			bottomPort.Deliver(rsp)

			// Tick: fetchFromBottom receives response, sends to top.
			modelingtest.Tick(gmmuComp)

			rspToTopI, _ := topPort.RetrieveOutgoing()
			require.NotNil(t, rspToTopI)
			rspToTop := rspToTopI
			require.Equal(t, page, rspToTop.Payload.(vmprotocol.TranslationRsp).Page)
			require.Equal(t, vm.PID(1), rspToTop.Payload.(vmprotocol.TranslationRsp).Page.PID)
		})
	})
}
