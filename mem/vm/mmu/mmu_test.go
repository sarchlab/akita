package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
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
func (c *noopConn) NotifySend()                      {}

// makePorts creates the ports of the MMU named name, with the given Top
// buffer size and a Control buffer size of 4.
func makePorts(name string, topBufSize int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(topBufSize, topBufSize),
		Control: twowaybuffered.NewPort(4, 4),
	}
}

var _ = Describe("MMU", func() {

	var (
		engine           timing.Engine
		sim              timing.Simulation
		pageTable        vm.PageTable
		mmuComp          *Comp
		topPort          messaging.Port
		translationMWRef *translationMW
	)

	// build constructs an MMU with the given Top buffer size, injects the
	// shared page table, and plugs noopConns so its ports can be driven.
	build := func(topBufSize int) {
		sim = modeling.NewStandaloneSimulation(engine)

		mmuComp = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(Definition.DefaultSpec).
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

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		pageTable = vm.NewPageTable(12)
		build(4096)
	})

	Context("parse top", func() {
		It("should process translation request", func() {
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
			Expect(next.WalkingTranslations).To(HaveLen(1))
		})

		It("should stall parse from top "+
			"if MMU is servicing max requests",
			func() {
				mmuComp.State = state{
					WalkingTranslations: make([]transactionState, 16),
				}

				madeProgress := translationMWRef.parseFromTop()

				Expect(madeProgress).To(BeFalse())
			})
	})

	Context("walk page table", func() {
		It("should reduce translation cycles", func() {
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
			Expect(next.WalkingTranslations[0].CycleLeft).To(Equal(9))
			Expect(madeProgress).To(BeTrue())
		})

		It("should send rsp to top if hit", func() {
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

			Expect(madeProgress).To(BeTrue())
			next := &mmuComp.State
			Expect(next.WalkingTranslations).To(HaveLen(0))

			rsp, _ := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: vmprotocol.TranslationRsp{}}))
			Expect(rsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))
		})

		It("should stall if cannot send to top", func() {
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

			dummy := messaging.Msg{Payload: vmprotocol.TranslationRsp{},
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

			Expect(madeProgress).To(BeFalse())
		})
	})

})

var _ = Describe("MMU Integration", func() {
	var (
		engine    timing.Engine
		sim       timing.Simulation
		mmuComp   *Comp
		pageTable vm.PageTable
		topPort   messaging.Port
		agentPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		pageTable = vm.NewPageTable(12)

		mmuComp = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(Definition.DefaultSpec).
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

	})

	It("should lookup", func() {
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
		Expect(rspI).ToNot(BeNil())
		rsp := rspI
		Expect(rsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))
		Expect(rsp.RspTo).To(Equal(req.ID))
	})
})

type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}
