package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("MMU", func() {

	var (
		engine           timing.Engine
		pageTable        vm.PageTable
		mmuComp          *Comp
		topPort          messaging.Port
		translationMWRef *translationMW
	)

	// build constructs an MMU with the given Top buffer size, injects the
	// shared page table, and plugs noopConns so its ports can be driven.
	build := func(topBufSize int) {
		spec := DefaultSpec()
		spec.TopPortBufferSize = topBufSize

		mmuComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("MMU")

		topPort = mmuComp.GetPortByName("Top")
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(mmuComp.GetPortByName("Control"))

		translationMWRef = mmuComp.Middlewares()[1].(*translationMW)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(12)
		build(4096)
	})

	Context("parse top", func() {
		It("should process translation request", func() {
			translationReq := vm.TranslationReq{}
			translationReq.ID = timing.GetIDGenerator().Generate()
			translationReq.Src = messaging.RemotePort("Agent")
			translationReq.Dst = topPort.AsRemote()
			translationReq.PID = 1
			translationReq.VAddr = 0x100000100
			translationReq.DeviceID = 0
			translationReq.TrafficClass = "vm.TranslationReq"
			topPort.Deliver(translationReq)

			translationMWRef.parseFromTop()

			next := &mmuComp.State
			Expect(next.WalkingTranslations).To(HaveLen(1))
		})

		It("should stall parse from top "+
			"if MMU is servicing max requests",
			func() {
				mmuComp.State = State{
					WalkingTranslations: make([]transactionState, 16),
				}

				madeProgress := translationMWRef.parseFromTop()

				Expect(madeProgress).To(BeFalse())
			})
	})

	Context("walk page table", func() {
		It("should reduce translation cycles", func() {
			mmuComp.State = State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     timing.GetIDGenerator().Generate(),
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

			mmuComp.State = State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     timing.GetIDGenerator().Generate(),
						ReqSrc:    messaging.RemotePort("Agent"),
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

			rsp := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(vm.TranslationRsp{}))
			Expect(rsp.(vm.TranslationRsp).Page).To(Equal(page))
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

			dummy := vm.TranslationRsp{}
			dummy.Src = topPort.AsRemote()
			dummy.Dst = messaging.RemotePort("Agent")
			dummy.TrafficClass = "vm.TranslationRsp"
			topPort.Send(dummy)

			mmuComp.State = State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     timing.GetIDGenerator().Generate(),
						ReqSrc:    messaging.RemotePort("Agent"),
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
		mmuComp   *Comp
		pageTable vm.PageTable
		topPort   messaging.Port
		agentPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		pageTable = vm.NewPageTable(12)

		mmuComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(DefaultSpec()).
			Build("MMU")

		topPort = mmuComp.GetPortByName("Top")
		(&noopConn{}).PlugIn(topPort)

		agentPort = messaging.NewPort(nil, 4, 4, "Agent")
		(&noopConn{}).PlugIn(agentPort)
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

		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agentPort.AsRemote()
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 0
		req.TrafficClass = "vm.TranslationReq"
		topPort.Deliver(req)

		// Drive enough ticks for the request to be parsed, walked, and
		// answered (default latency is 10).
		for i := 0; i < 20; i++ {
			mmuComp.Tick()
		}

		rspI := topPort.RetrieveOutgoing()
		Expect(rspI).ToNot(BeNil())
		rsp := rspI.(vm.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))
		Expect(rsp.RspTo).To(Equal(req.ID))
	})
})
