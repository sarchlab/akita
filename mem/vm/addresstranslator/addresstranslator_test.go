package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
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

var _ = Describe("Address Translator", func() {
	var (
		engine          timing.Engine
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
		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1
		spec.TopPortBufferSize = topBufSize

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		t = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")

		topPort = t.GetPortByName("Top")
		bottomPort = t.GetPortByName("Bottom")
		translationPort = t.GetPortByName("Translation")
		ctrlPort = t.GetPortByName("Control")

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}

		tParseTransMW = t.Middlewares()[0].(*parseTranslateMW)
		tRespondPipeMW = t.Middlewares()[1].(*respondPipelineMW)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build(4)
	})

	Context("translate stage", func() {
		var (
			req *mem.ReadReq
		)

		BeforeEach(func() {
			req = &mem.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.Dst = topPort.AsRemote()
			req.Address = 0x100
			req.AccessByteSize = 4
			req.PID = 1
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
		})

		It("should do nothing if there is no request", func() {
			madeProgress := tParseTransMW.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			req.Address = 0x1040
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(1))

			sent := translationPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(&vm.TranslationReq{}))
			transReq := sent.(*vm.TranslationReq)
			Expect(updatedState.Transactions[0].TranslationReqID).
				To(Equal(transReq.ID))
		})

		It("should stall if cannot send for translation", func() {
			// Fill the translation port's outgoing buffer so Send fails.
			fillOutgoing(translationPort, t.Spec().TranslationPortBufferSize)
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(0))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 *vm.TranslationReq
		)

		BeforeEach(func() {
			transReq1 = &vm.TranslationReq{}
			transReq1.ID = timing.GetIDGenerator().Generate()
			transReq1.PID = 1
			transReq1.VAddr = 0x100
			transReq1.DeviceID = 1
			transReq1.TrafficClass = "vm.TranslationReq"
			transReq2 = &vm.TranslationReq{}
			transReq2.ID = timing.GetIDGenerator().Generate()
			transReq2.PID = 1
			transReq2.VAddr = 0x100
			transReq2.DeviceID = 1
			transReq2.TrafficClass = "vm.TranslationReq"

			t.State = State{
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
			req := &mem.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
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
			fillOutgoing(bottomPort, t.Spec().BottomPortBufferSize)
			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := &mem.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
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

			sent := bottomPort.RetrieveOutgoing()
			read := sent.(*mem.ReadReq)
			Expect(read.PID).To(Equal(vm.PID(0)))
			Expect(read.Address).To(Equal(uint64(0x20040)))
			Expect(read.AccessByteSize).To(Equal(uint64(4)))
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
			write := &mem.WriteReq{}
			write.ID = timing.GetIDGenerator().Generate()
			write.Address = 0x10040
			write.Data = data
			write.DirtyMask = dirty
			write.TrafficBytes = len(data) + 12
			write.TrafficClass = "mem.WriteReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
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

			sent := bottomPort.RetrieveOutgoing()
			writeMsg := sent.(*mem.WriteReq)
			Expect(writeMsg.PID).To(Equal(vm.PID(0)))
			Expect(writeMsg.Address).To(Equal(uint64(0x20040)))
			Expect(writeMsg.Src).To(Equal(bottomPort.AsRemote()))
			Expect(writeMsg.Data).To(Equal(data))
			Expect(writeMsg.DirtyMask).To(Equal(dirty))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})
	})

	Context("respond", func() {
		var (
			readFromTop   *mem.ReadReq
			writeFromTop  *mem.WriteReq
			readToBottom  *mem.ReadReq
			writeToBottom *mem.WriteReq
		)

		BeforeEach(func() {
			readFromTop = &mem.ReadReq{}
			readFromTop.ID = timing.GetIDGenerator().Generate()
			readFromTop.Src = messaging.RemotePort("Agent")
			readFromTop.Dst = topPort.AsRemote()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Src = bottomPort.AsRemote()
			readToBottom.Dst = messaging.RemotePort("MemPort")
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = timing.GetIDGenerator().Generate()
			writeFromTop.Src = messaging.RemotePort("Agent")
			writeFromTop.Dst = topPort.AsRemote()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = timing.GetIDGenerator().Generate()
			writeToBottom.Src = bottomPort.AsRemote()
			writeToBottom.Dst = messaging.RemotePort("MemPort")
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"

			t.State = State{
				InflightReqToBottom: []reqToBottomState{
					{
						ReqFromTopID:    readFromTop.ID,
						ReqFromTopSrc:   readFromTop.Src,
						ReqFromTopDst:   readFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", readFromTop),
						ReqToBottomID:   readToBottom.ID,
						ReqToBottomSrc:  readToBottom.Src,
						ReqToBottomDst:  readToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", readToBottom),
					},
					{
						ReqFromTopID:    writeFromTop.ID,
						ReqFromTopSrc:   writeFromTop.Src,
						ReqFromTopDst:   writeFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", writeFromTop),
						ReqToBottomID:   writeToBottom.ID,
						ReqToBottomSrc:  writeToBottom.Src,
						ReqToBottomDst:  writeToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", writeToBottom),
					},
				},
			}
		})

		It("should do nothing if there is no response to process", func() {
			madeProgress := tRespondPipeMW.respond()
			Expect(madeProgress).To(BeFalse())
		})

		It("should respond data ready", func() {
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent := topPort.RetrieveOutgoing()
			dr := sent.(*mem.DataReadyRsp)
			Expect(dr.RspTo).To(Equal(readFromTop.ID))
			Expect(dr.Data).To(Equal(dataReady.Data))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := &mem.WriteDoneRsp{}
			done.ID = timing.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "mem.WriteDoneRsp"
			bottomPort.Deliver(done)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent := topPort.RetrieveOutgoing()
			doneMsg := sent.(*mem.WriteDoneRsp)
			Expect(doneMsg.RspTo).To(Equal(writeFromTop.ID))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"

			// Fill the top port's outgoing buffer so Send fails.
			fillOutgoing(topPort, t.Spec().TopPortBufferSize)
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("state serialization", func() {
		It("should pass ValidateState", func() {
			err := modeling.ValidateState(State{})
			Expect(err).To(Succeed())
		})
	})

	Context("when handling control messages", func() {
		var (
			readFromTop   *mem.ReadReq
			writeFromTop  *mem.WriteReq
			readToBottom  *mem.ReadReq
			writeToBottom *mem.WriteReq
			flushReq      *mem.ControlReq
			restartReq    *mem.ControlReq
		)

		BeforeEach(func() {
			readFromTop = &mem.ReadReq{}
			readFromTop.ID = timing.GetIDGenerator().Generate()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = timing.GetIDGenerator().Generate()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = timing.GetIDGenerator().Generate()
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"
			flushReq = &mem.ControlReq{
				Command: mem.CmdFlush,
			}
			flushReq.ID = timing.GetIDGenerator().Generate()
			flushReq.Src = messaging.RemotePort("Agent")
			flushReq.Dst = ctrlPort.AsRemote()
			flushReq.TrafficBytes = 4
			flushReq.TrafficClass = "mem.ControlReq"
			restartReq = &mem.ControlReq{
				Command: mem.CmdReset,
			}
			restartReq.ID = timing.GetIDGenerator().Generate()
			restartReq.Src = messaging.RemotePort("Agent")
			restartReq.Dst = ctrlPort.AsRemote()
			restartReq.TrafficBytes = 4
			restartReq.TrafficClass = "mem.ControlReq"

			nextState := &t.State
			nextState.InflightReqToBottom = []reqToBottomState{
				{
					ReqFromTopID:    readFromTop.ID,
					ReqFromTopSrc:   readFromTop.Src,
					ReqFromTopDst:   readFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", readFromTop),
					ReqToBottomID:   readToBottom.ID,
					ReqToBottomSrc:  readToBottom.Src,
					ReqToBottomDst:  readToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", readToBottom),
				},
				{
					ReqFromTopID:    writeFromTop.ID,
					ReqFromTopSrc:   writeFromTop.Src,
					ReqFromTopDst:   writeFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", writeFromTop),
					ReqToBottomID:   writeToBottom.ID,
					ReqToBottomSrc:  writeToBottom.Src,
					ReqToBottomDst:  writeToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", writeToBottom),
				},
			}
		})

		It("should handle flush req", func() {
			ctrlPort.Deliver(flushReq)

			madeProgress := tParseTransMW.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			Expect(ctrlPort.RetrieveOutgoing()).To(
				BeAssignableToTypeOf(&mem.ControlRsp{}))
			updatedState := &t.State
			Expect(updatedState.IsFlushing).To(BeTrue())
			Expect(updatedState.InflightReqToBottom).To(BeNil())
		})

		It("should handle restart req", func() {
			ctrlPort.Deliver(restartReq)

			madeProgress := tParseTransMW.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			Expect(ctrlPort.RetrieveOutgoing()).To(
				BeAssignableToTypeOf(&mem.ControlRsp{}))
			updatedState := &t.State
			Expect(updatedState.IsFlushing).To(BeFalse())
		})

	})
})

// fillOutgoing fills a port's outgoing buffer with dummy messages so the next
// CanSend returns false. Each dummy's Src equals the port (required by Send's
// validation) and is sent to a distinct destination.
func fillOutgoing(p messaging.Port, n int) {
	for i := 0; i < n; i++ {
		dummy := &mem.WriteDoneRsp{}
		dummy.ID = timing.GetIDGenerator().Generate()
		dummy.Src = p.AsRemote()
		dummy.Dst = messaging.RemotePort("Dummy")
		dummy.TrafficClass = "mem.WriteDoneRsp"
		Expect(p.CanSend()).To(BeTrue())
		p.Send(dummy)
	}
}
