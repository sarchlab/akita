package gmmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("GMMU", func() {
	var (
		engine     timing.Engine
		sim        timing.Simulation
		pageTable  vm.PageTable
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
		spec := Definition.DefaultSpec
		spec.DeviceID = 0
		spec.Latency = 1
		spec.LowModule = lowModulePort

		gmmuComp = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			WithPorts(defaultPorts("MMU")).
			Build("MMU")

		mw = gmmuComp.Middlewares.Walk

		topPort = gmmuComp.Ports.Top
		bottomPort = gmmuComp.Ports.Bottom

		topConn := &noopConn{}
		topConn.PlugIn(topPort)
		bottomConn := &noopConn{}
		bottomConn.PlugIn(bottomPort)
		(&noopConn{}).PlugIn(gmmuComp.Ports.Control)
	}

	makeTranslationReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = sim.NewID()
		req.Src = agentPort
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		pageTable = vm.NewPageTable(12)
		build()
	})

	Context("GMMU Builder", func() {
		It("should build GMMU correctly", func() {
			Expect(gmmuComp.Spec.Freq).To(Equal(1 * timing.GHz))
			Expect(gmmuComp.Spec.MaxRequestsInFlight).To(Equal(16))
			Expect(mw.pageTable).To(Equal(pageTable))
			Expect(gmmuComp.Spec.DeviceID).To(Equal(uint64(0)))
		})
	})

	Context("GMMU parse from top", func() {
		It("should process translation request", func() {
			topPort.Deliver(makeTranslationReq(0x00000000))

			mw.Handle(modelingtest.TickEvent(gmmuComp))

			state := &gmmuComp.State
			Expect(state.WalkingTranslations).To(HaveLen(1))
		})

		It("should walk page table", func() {
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
			Expect(rspI).NotTo(BeNil())
			rsp := rspI.(vmprotocol.TranslationRsp)
			Expect(rsp.Page).To(Equal(page))
			Expect(rsp.Page.PID).To(Equal(vm.PID(1)))
		})

		It("should send request remotely", func() {
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
			Expect(reqI).NotTo(BeNil())
			req := reqI.(vmprotocol.TranslationReq)
			Expect(req.Dst).To(Equal(lowModulePort))
			Expect(req.VAddr).To(Equal(uint64(0x10000000)))
		})

		It("should return response from remote page table", func() {
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
			Expect(reqI).NotTo(BeNil())
			sentReqToBottom := reqI.(vmprotocol.TranslationReq)

			// Deliver the response from the bottom (remote page table).
			rsp := vmprotocol.TranslationRsp{
				Page: page,
			}
			rsp.ID = sim.NewID()
			rsp.Src = lowModulePort
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = sentReqToBottom.ID
			rsp.TrafficClass = "vmprotocol.TranslationRsp"
			bottomPort.Deliver(rsp)

			// Tick: fetchFromBottom receives response, sends to top.
			modelingtest.Tick(gmmuComp)

			rspToTopI, _ := topPort.RetrieveOutgoing()
			Expect(rspToTopI).NotTo(BeNil())
			rspToTop := rspToTopI.(vmprotocol.TranslationRsp)
			Expect(rspToTop.Page).To(Equal(page))
			Expect(rspToTop.Page.PID).To(Equal(vm.PID(1)))
		})
	})
})
