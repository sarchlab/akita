package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the translator now owns its ports (they are no
// longer injectable), tests feed requests with Deliver and read responses with
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

// Historical default port buffer sizes for the address translator. Buffer size
// is now the caller's choice (the Spec no longer carries *PortBufferSize
// fields), so tests own these constants.
const (
	topBufSize         = 4
	bottomBufSize      = 4
	translationBufSize = 4
	ctrlBufSize        = 1
)

// makePorts creates the four ports (Top, Bottom, Translation, Control) of the
// translator named name, with the given Top buffer size and the historical
// defaults for the rest.
func makePorts(name string, topBufSize int) Ports {
	return Ports{
		Top:         twowaybuffered.NewPort(name+".Top", topBufSize, topBufSize),
		Bottom:      twowaybuffered.NewPort(name+".Bottom", bottomBufSize, bottomBufSize),
		Translation: twowaybuffered.NewPort(name+".Translation", translationBufSize, translationBufSize),
		Control:     twowaybuffered.NewPort(name+".Control", ctrlBufSize, ctrlBufSize),
	}
}

var _ = Describe("Address Translator", func() {
	var (
		engine          timing.Engine
		sim             timing.Simulation
		t               *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
		tParseTransMW   *parseTranslateMW
		tRespondPipeMW  *respondPipelineMW
	)

	// build constructs a translator with the given Top-port buffer size, injects
	// the mappers, and plugs a noopConn into each port so they can be driven.
	build := func(topBufSize int) {
		spec := Definition.DefaultSpec
		spec.Log2PageSize = 12
		spec.Freq = 1

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		t = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(resources).
			WithPorts(makePorts("AddressTranslator", topBufSize)).
			Build("AddressTranslator")

		topPort = t.Ports.Top
		bottomPort = t.Ports.Bottom
		translationPort = t.Ports.Translation
		ctrlPort = t.Ports.Control

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}

		tParseTransMW = t.Middlewares.ParseTranslate
		tRespondPipeMW = t.Middlewares.RespondPipeline
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		build(topBufSize)
	})

	Context("translate stage", func() {
		var (
			req messaging.Msg
		)

		BeforeEach(func() {
			req = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x100,
				AccessByteSize: 4,
				PID:            1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

		})

		It("should do nothing if there is no request", func() {
			madeProgress := tParseTransMW.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			payload1 := req.Payload.(memprotocol.ReadReq)
			payload1.Address = 0x1040
			req.Payload = payload1
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(1))

			sent, _ := translationPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(messaging.Msg{Payload: vmprotocol.TranslationReq{}}))
			transReq := sent
			Expect(updatedState.Transactions[0].TranslationReqID).
				To(Equal(transReq.ID))
		})

		It("should stall if cannot send for translation", func() {
			// Fill the translation port's outgoing buffer so Send fails.
			fillOutgoing(translationPort, translationBufSize)
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(0))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 messaging.Msg
		)

		BeforeEach(func() {
			transReq1 = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),

				TrafficClass: "vmprotocol.TranslationReq"}

			transReq2 = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),

				TrafficClass: "vmprotocol.TranslationReq"}

			t.State = state{
				Transactions: []transactionState{
					{TranslationReqID: transReq1.ID},
					{TranslationReqID: transReq2.ID},
				},
			}
		})

		It("should do nothing if there is no translation return", func() {
			needTick := tRespondPipeMW.parseTranslation()
			Expect(needTick).To(BeFalse())
		})

		It("should stall if send failed", func() {
			req := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:           sim.NewID(),
				RspTo:        transReq1.ID,
				TrafficClass: "vmprotocol.TranslationRsp"}

			t.State = state{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			// Fill the bottom port's outgoing buffer so Send fails.
			fillOutgoing(bottomPort, bottomBufSize)
			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:           sim.NewID(),
				RspTo:        transReq1.ID,
				TrafficClass: "vmprotocol.TranslationRsp"}

			t.State = state{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())

			sent, _ := bottomPort.RetrieveOutgoing()
			read := sent
			Expect(read.Payload.(memprotocol.ReadReq).PID).To(Equal(vm.PID(0)))
			Expect(read.Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0x20040)))
			Expect(read.Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(4)))
			Expect(read.Src).To(Equal(bottomPort.AsRemote()))

			updatedState := &t.State
			Expect(updatedState.Transactions).NotTo(
				ContainElement(
					WithTransform(
						func(ts transactionState) uint64 { return ts.TranslationReqID },
						Equal(transReq1.ID),
					),
				),
			)
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should forward write request", func() {
			data := []byte{1, 2, 3, 4}
			dirty := []bool{false, true, false, true}
			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address:   0x10040,
				Data:      data,
				DirtyMask: dirty},
				ID: sim.NewID(),

				TrafficBytes: len(data) + 12,
				TrafficClass: "memprotocol.WriteReq"}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:           sim.NewID(),
				RspTo:        transReq1.ID,
				TrafficClass: "vmprotocol.TranslationRsp"}

			t.State = state{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(write),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())

			sent, _ := bottomPort.RetrieveOutgoing()
			writeMsg := sent
			Expect(writeMsg.Payload.(memprotocol.WriteReq).PID).To(Equal(vm.PID(0)))
			Expect(writeMsg.Payload.(memprotocol.WriteReq).Address).To(Equal(uint64(0x20040)))
			Expect(writeMsg.Src).To(Equal(bottomPort.AsRemote()))
			Expect(writeMsg.Payload.(memprotocol.WriteReq).Data).To(Equal(data))
			Expect(writeMsg.Payload.(memprotocol.WriteReq).DirtyMask).To(Equal(dirty))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})
	})

	Context("respond", func() {
		var (
			readFromTop   messaging.Msg
			writeFromTop  messaging.Msg
			readToBottom  messaging.Msg
			writeToBottom messaging.Msg
		)

		BeforeEach(func() {
			readFromTop = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			readToBottom = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x20040,
				AccessByteSize: 4},
				ID:  sim.NewID(),
				Src: bottomPort.AsRemote(),
				Dst: messaging.RemotePort("MemPort"),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			writeFromTop = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.WriteReq"}

			writeToBottom = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID:  sim.NewID(),
				Src: bottomPort.AsRemote(),
				Dst: messaging.RemotePort("MemPort"),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.WriteReq"}

			t.State = state{
				InflightReqToBottom: []reqToBottomState{
					{
						ReqFromTopID:    readFromTop.ID,
						ReqFromTopSrc:   readFromTop.Src,
						ReqFromTopDst:   readFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", readFromTop.Payload),
						ReqToBottomID:   readToBottom.ID,
						ReqToBottomSrc:  readToBottom.Src,
						ReqToBottomDst:  readToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", readToBottom.Payload),
					},
					{
						ReqFromTopID:    writeFromTop.ID,
						ReqFromTopSrc:   writeFromTop.Src,
						ReqFromTopDst:   writeFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", writeFromTop.Payload),
						ReqToBottomID:   writeToBottom.ID,
						ReqToBottomSrc:  writeToBottom.Src,
						ReqToBottomDst:  writeToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", writeToBottom.Payload),
					},
				},
			}
		})

		It("should do nothing if there is no response to process", func() {
			madeProgress := tRespondPipeMW.respond()
			Expect(madeProgress).To(BeFalse())
		})

		It("should respond data ready", func() {
			dataReady := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
				ID:           sim.NewID(),
				RspTo:        readToBottom.ID,
				TrafficBytes: 4,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent, _ := topPort.RetrieveOutgoing()
			dr := sent
			Expect(dr.RspTo).To(Equal(readFromTop.ID))
			Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal(dataReady.Payload.(memprotocol.DataReadyRsp).Data))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           sim.NewID(),
				RspTo:        writeToBottom.ID,
				TrafficBytes: 4,
				TrafficClass: "memprotocol.WriteDoneRsp"}

			bottomPort.Deliver(done)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent, _ := topPort.RetrieveOutgoing()
			doneMsg := sent
			Expect(doneMsg.RspTo).To(Equal(writeFromTop.ID))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
				ID:           sim.NewID(),
				RspTo:        readToBottom.ID,
				TrafficBytes: 4,
				TrafficClass: "memprotocol.DataReadyRsp"}

			// Fill the top port's outgoing buffer so Send fails.
			fillOutgoing(topPort, topBufSize)
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("state serialization", func() {
		It("should pass ValidateState", func() {
			err := modeling.ValidateState(state{})
			Expect(err).To(Succeed())
		})
	})

	Context("when handling control messages", func() {
		var (
			readFromTop   messaging.Msg
			writeFromTop  messaging.Msg
			readToBottom  messaging.Msg
			writeToBottom messaging.Msg
			flushReq      messaging.Msg
			restartReq    messaging.Msg
		)

		BeforeEach(func() {
			readFromTop = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			readToBottom = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x20040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			writeFromTop = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.WriteReq"}

			writeToBottom = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID: sim.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.WriteReq"}

			flushReq = messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdFlush,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          ctrlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			restartReq = messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdReset,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          ctrlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			nextState := &t.State
			nextState.InflightReqToBottom = []reqToBottomState{
				{
					ReqFromTopID:    readFromTop.ID,
					ReqFromTopSrc:   readFromTop.Src,
					ReqFromTopDst:   readFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", readFromTop.Payload),
					ReqToBottomID:   readToBottom.ID,
					ReqToBottomSrc:  readToBottom.Src,
					ReqToBottomDst:  readToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", readToBottom.Payload),
				},
				{
					ReqFromTopID:    writeFromTop.ID,
					ReqFromTopSrc:   writeFromTop.Src,
					ReqFromTopDst:   writeFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", writeFromTop.Payload),
					ReqToBottomID:   writeToBottom.ID,
					ReqToBottomSrc:  writeToBottom.Src,
					ReqToBottomDst:  writeToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", writeToBottom.Payload),
				},
			}
		})

		It("rejects Flush as unsupported", func() {
			ctrlPort.Deliver(flushReq)

			madeProgress := modelingtest.Tick(t)

			Expect(madeProgress).To(BeTrue())
			rspMsg, _ := ctrlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
			rsp := rspMsg
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeFalse())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Error).To(Equal(memcontrolprotocol.ErrUnsupported))
		})

		It("clears in-flight state on Reset", func() {
			ctrlPort.Deliver(restartReq)

			madeProgress := modelingtest.Tick(t)

			Expect(madeProgress).To(BeTrue())
			rspMsg, _ := ctrlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
			rsp := rspMsg
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
			Expect(updatedState.InflightReqToBottom).To(BeEmpty())
		})

	})
})

// fillOutgoing fills a port's outgoing buffer with dummy messages so the next
// CanSend returns false. Each dummy's Src equals the port (required by Send's
// validation) and is sent to a distinct destination.
func fillOutgoing(p messaging.Port, n int) {
	for i := 0; i < n; i++ {
		dummy := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:           p.Owner().(interface{ NewID() uint64 }).NewID(),
			Src:          p.AsRemote(),
			Dst:          messaging.RemotePort("Dummy"),
			TrafficClass: "memprotocol.WriteDoneRsp"}

		Expect(p.CanSend()).To(BeTrue())
		p.Send(dummy)
	}
}
