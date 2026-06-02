package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("MMUCacheCtrlMiddleware", func() {
	var (
		engine      timing.Engine
		comp        *Comp
		ctrl        *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("MMUCache")
		comp.State.CurrentState = mmuCacheStatePause

		topPort = comp.GetPortByName("Top")
		bottomPort = comp.GetPortByName("Bottom")
		controlPort = comp.GetPortByName("Control")
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		ctrl = &ctrlMiddleware{comp: comp}
	})

	It("should do nothing when no control message", func() {
		madeProgress := ctrl.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should restart and drain ports", func() {
		req := &mem.ControlReq{Command: mem.CmdReset}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.TrafficClass = "mem.ControlReq"

		topMsg := &vm.TranslationReq{}
		topMsg.ID = timing.GetIDGenerator().Generate()
		topMsg.Src = messaging.RemotePort("Requester")
		topMsg.Dst = topPort.AsRemote()
		topMsg.PID = 1
		topMsg.VAddr = 0x1000
		topMsg.DeviceID = 1
		topMsg.TrafficClass = "vm.TranslationReq"
		Expect(topPort.Deliver(topMsg)).To(BeNil())

		bottomMsg := &vm.TranslationRsp{
			Page: vm.Page{},
		}
		bottomMsg.ID = timing.GetIDGenerator().Generate()
		bottomMsg.Src = messaging.RemotePort("LowModule")
		bottomMsg.Dst = bottomPort.AsRemote()
		bottomMsg.RspTo = timing.GetIDGenerator().Generate()
		bottomMsg.TrafficClass = "vm.TranslationRsp"
		Expect(bottomPort.Deliver(bottomMsg)).To(BeNil())

		madeProgress := ctrl.handleMMUCacheRestart(req)

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStateEnable))
		Expect(topPort.PeekIncoming()).To(BeNil())
		Expect(bottomPort.PeekIncoming()).To(BeNil())

		rsp := controlPort.RetrieveOutgoing()
		ctrlRsp, ok := rsp.(*mem.ControlRsp)
		Expect(ok).To(BeTrue())
		Expect(ctrlRsp.Command).To(Equal(mem.CmdReset))
		Expect(ctrlRsp.Success).To(BeTrue())
		Expect(ctrlRsp.Dst).To(Equal(messaging.RemotePort("Requester")))
		Expect(ctrlRsp.Src).To(Equal(controlPort.AsRemote()))
	})

	It("should accept flush request in enable state", func() {
		spec := comp.Spec()
		comp.State = State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		req := &mem.ControlReq{Command: mem.CmdFlush}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = controlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		Expect(controlPort.Deliver(req)).To(BeNil())

		madeProgress := ctrl.handleMMUCacheFlush(req)

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.InflightFlushReqActive).To(BeTrue())
		Expect(next.InflightFlushReqID).To(Equal(req.ID))
		Expect(next.InflightFlushReqSrc).To(Equal(req.Src))
		Expect(next.CurrentState).To(Equal(mmuCacheStateFlush))
	})

	It("should handle control pause", func() {
		spec := comp.Spec()
		comp.State = State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		msg := &mem.ControlReq{
			Command: mem.CmdPause,
		}
		msg.ID = timing.GetIDGenerator().Generate()
		msg.Src = messaging.RemotePort("Requester")
		msg.Dst = controlPort.AsRemote()
		msg.TrafficBytes = 4
		msg.TrafficClass = "mem.ControlReq"
		Expect(controlPort.Deliver(msg)).To(BeNil())

		madeProgress := ctrl.handleIncomingCommands()

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
	})
})
