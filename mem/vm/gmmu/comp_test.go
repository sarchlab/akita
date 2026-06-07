package gmmu

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

// assignPort builds a port with the given buffer size using the same registrar
// the component was built with, and assigns it to the component's declared port
// of the same name.
func assignPort(
	reg modeling.Registrar,
	comp *Comp,
	name string,
	bufSize int,
) messaging.Port {
	p := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, p)
	return p
}

// assignDefaultPorts assigns the GMMU's three declared ports (Top, Bottom,
// Control) with the historical default buffer sizes.
func assignDefaultPorts(reg modeling.Registrar, comp *Comp) {
	assignPort(reg, comp, "Top", 16)
	assignPort(reg, comp, "Bottom", 16)
	assignPort(reg, comp, "Control", 4)
}

var _ = Describe("GMMU", func() {
	var (
		engine     timing.Engine
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
		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.LowModule = lowModulePort

		reg := modeling.NewStandaloneRegistrar(engine)
		gmmuComp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("MMU")

		assignDefaultPorts(reg, gmmuComp)

		mw = gmmuComp.Middlewares()[1].(*walkMW)

		topPort = gmmuComp.GetPortByName("Top")
		bottomPort = gmmuComp.GetPortByName("Bottom")

		topConn := &noopConn{}
		topConn.PlugIn(topPort)
		bottomConn := &noopConn{}
		bottomConn.PlugIn(bottomPort)
		(&noopConn{}).PlugIn(gmmuComp.GetPortByName("Control"))
	}

	makeTranslationReq := func(vAddr uint64) vm.TranslationReq {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agentPort
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
		req.TrafficClass = "vm.TranslationReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(12)
		build()
	})

	Context("GMMU Builder", func() {
		It("should build GMMU correctly", func() {
			Expect(gmmuComp.Spec().Freq).To(Equal(1 * timing.GHz))
			Expect(gmmuComp.Spec().MaxRequestsInFlight).To(Equal(16))
			Expect(mw.pageTable).To(Equal(pageTable))
			Expect(gmmuComp.Spec().DeviceID).To(Equal(uint64(0)))
		})
	})

	Context("GMMU parse from top", func() {
		It("should process translation request", func() {
			topPort.Deliver(makeTranslationReq(0x00000000))

			mw.Tick()

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
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees the translation and decrements
			// CycleLeft (latency=1 → 0).
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page walk completes and sends response.
			gmmuComp.Tick()

			rspI := topPort.RetrieveOutgoing()
			Expect(rspI).NotTo(BeNil())
			rsp := rspI.(vm.TranslationRsp)
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
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			gmmuComp.Tick()

			reqI := bottomPort.RetrieveOutgoing()
			Expect(reqI).NotTo(BeNil())
			req := reqI.(vm.TranslationReq)
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
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			gmmuComp.Tick()

			reqI := bottomPort.RetrieveOutgoing()
			Expect(reqI).NotTo(BeNil())
			sentReqToBottom := reqI.(vm.TranslationReq)

			// Deliver the response from the bottom (remote page table).
			rsp := vm.TranslationRsp{
				Page: page,
			}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = lowModulePort
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = sentReqToBottom.ID
			rsp.TrafficClass = "vm.TranslationRsp"
			bottomPort.Deliver(rsp)

			// Tick: fetchFromBottom receives response, sends to top.
			gmmuComp.Tick()

			rspToTopI := topPort.RetrieveOutgoing()
			Expect(rspToTopI).NotTo(BeNil())
			rspToTop := rspToTopI.(vm.TranslationRsp)
			Expect(rspToTop.Page).To(Equal(page))
			Expect(rspToTop.Page.PID).To(Equal(vm.PID(1)))
		})
	})
})
