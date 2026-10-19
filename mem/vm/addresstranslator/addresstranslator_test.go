package addresstranslator

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
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
// ports in isolation. Because the translator now owns its ports (they are no
// longer injectable), tests feed requests with Deliver and read responses with
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
func makePorts(simulation modeling.Simulation, name string, topBufSize int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: topBufSize, OutgoingCapacity: topBufSize}).
			Build(),
		Bottom: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bottomBufSize, OutgoingCapacity: bottomBufSize}).
			Build(),
		Translation: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: translationBufSize, OutgoingCapacity: translationBufSize}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: ctrlBufSize, OutgoingCapacity: ctrlBufSize}).
			Build(),
	}
}

func TestAddressTranslator(t *testing.T) {
	var (
		engine          timing.Engine
		sim             *testsim.Simulation
		translator      *Comp
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

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("MemPort")}
		spec.Translation.Destinations = []messaging.RemotePort{messaging.RemotePort("TranslationProvider")}

		translator = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("AddressTranslator")
		tPorts := makePorts(sim, "AddressTranslator", topBufSize)
		translator.BindPort("Top", tPorts.Top)
		translator.BindPort("Bottom", tPorts.Bottom)
		translator.BindPort("Translation", tPorts.Translation)
		translator.BindPort("Control", tPorts.Control)

		topPort = translator.Ports.Top
		bottomPort = translator.Ports.Bottom
		translationPort = translator.Ports.Translation
		ctrlPort = translator.Ports.Control

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		tParseTransMW = translator.Middlewares.ParseTranslate
		tRespondPipeMW = translator.Middlewares.RespondPipeline
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build(topBufSize)
	}

	t.Run("translate stage", func(t *testing.T) {
		var (
			req messaging.Msg
		)

		setup2 := func(t *testing.T) {
			t.Helper()

			req = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x100,
				AccessByteSize: 4,
				PID:            1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
			}
		}

		t.Run("should do nothing if there is no request", func(t *testing.T) {
			setup1(t)
			setup2(t)

			madeProgress := tParseTransMW.translate()
			require.False(t, madeProgress)
		})

		t.Run("should send translation", func(t *testing.T) {
			setup1(t)
			setup2(t)

			payload1 := req.Payload.(memprotocol.ReadReq)
			payload1.Address = 0x1040
			req.Payload = payload1
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			require.True(t, needTick)
			updatedState := &translator.State
			require.Len(t, updatedState.Transactions, 1)

			sent, _ := translationPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: vmprotocol.TranslationReq{}}, sent)
			transReq := sent
			require.Equal(t, transReq.ID, updatedState.Transactions[0].TranslationReqID)
		})

		t.Run("should stall if cannot send for translation", func(t *testing.T) {
			setup1(t)
			setup2(t)

			// Fill the translation port's outgoing buffer so Send fails.
			fillOutgoing(t, translationPort, translationBufSize)
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			require.False(t, needTick)
			updatedState := &translator.State
			require.Len(t, updatedState.Transactions, 0)
		})
	})

	t.Run("parse translation", func(t *testing.T) {
		var (
			transReq1, transReq2 messaging.Msg
		)

		setup3 := func(t *testing.T) {
			t.Helper()

			transReq1 = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),
			}

			transReq2 = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),
			}

			translator.State = state{
				Transactions: []transactionState{
					{TranslationReqID: transReq1.ID},
					{TranslationReqID: transReq2.ID},
				},
			}
		}

		t.Run("should do nothing if there is no translation return", func(t *testing.T) {
			setup1(t)
			setup3(t)

			needTick := tRespondPipeMW.parseTranslation()
			require.False(t, needTick)
		})

		t.Run("should stall if send failed", func(t *testing.T) {
			setup1(t)
			setup3(t)

			req := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:    sim.NewID(),
				RspTo: transReq1.ID,
			}

			translator.State = state{
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
			fillOutgoing(t, bottomPort, bottomBufSize)
			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			require.False(t, madeProgress)
		})

		t.Run("should forward read request", func(t *testing.T) {
			setup1(t)
			setup3(t)

			req := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:    sim.NewID(),
				RspTo: transReq1.ID,
			}

			translator.State = state{
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

			require.True(t, madeProgress)

			sent, _ := bottomPort.RetrieveOutgoing()
			read := sent
			require.Equal(t, vm.PID(0), read.Payload.(memprotocol.ReadReq).PID)
			require.Equal(t, uint64(0x20040), read.Payload.(memprotocol.ReadReq).Address)
			require.Equal(t, uint64(4), read.Payload.(memprotocol.ReadReq).AccessByteSize)
			require.Equal(t, bottomPort.AsRemote(), read.Src)

			updatedState := &translator.State
			for _, transaction := range updatedState.Transactions {
				require.NotEqual(t, transReq1.ID, transaction.TranslationReqID)
			}
			require.Len(t, updatedState.InflightReqToBottom, 1)
		})

		t.Run("should forward write request", func(t *testing.T) {
			setup1(t)
			setup3(t)

			data := []byte{1, 2, 3, 4}
			dirty := []bool{false, true, false, true}
			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address:   0x10040,
				Data:      data,
				DirtyMask: dirty},
				ID: sim.NewID(),

				TrafficBytes: len(data) + 12,
			}

			translationRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			},
				ID:    sim.NewID(),
				RspTo: transReq1.ID,
			}

			translator.State = state{
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

			require.True(t, madeProgress)

			sent, _ := bottomPort.RetrieveOutgoing()
			writeMsg := sent
			require.Equal(t, vm.PID(0), writeMsg.Payload.(memprotocol.WriteReq).PID)
			require.Equal(t, uint64(0x20040), writeMsg.Payload.(memprotocol.WriteReq).Address)
			require.Equal(t, bottomPort.AsRemote(), writeMsg.Src)
			require.Equal(t, data, writeMsg.Payload.(memprotocol.WriteReq).Data)
			require.Equal(t, dirty, writeMsg.Payload.(memprotocol.WriteReq).DirtyMask)

			updatedState := &translator.State
			require.Len(t, updatedState.InflightReqToBottom, 1)
		})
	})

	t.Run("respond", func(t *testing.T) {
		var (
			readFromTop   messaging.Msg
			writeFromTop  messaging.Msg
			readToBottom  messaging.Msg
			writeToBottom messaging.Msg
		)

		setup4 := func(t *testing.T) {
			t.Helper()

			readFromTop = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
			}

			readToBottom = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x20040,
				AccessByteSize: 4},
				ID:  sim.NewID(),
				Src: bottomPort.AsRemote(),
				Dst: messaging.RemotePort("MemPort"),

				TrafficBytes: 12,
			}

			writeFromTop = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: topPort.AsRemote(),

				TrafficBytes: 12,
			}

			writeToBottom = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID:  sim.NewID(),
				Src: bottomPort.AsRemote(),
				Dst: messaging.RemotePort("MemPort"),

				TrafficBytes: 12,
			}

			translator.State = state{
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
		}

		t.Run("should do nothing if there is no response to process", func(t *testing.T) {
			setup1(t)
			setup4(t)

			madeProgress := tRespondPipeMW.respond()
			require.False(t, madeProgress)
		})

		t.Run("should respond data ready", func(t *testing.T) {
			setup1(t)
			setup4(t)

			dataReady := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
				ID:           sim.NewID(),
				RspTo:        readToBottom.ID,
				TrafficBytes: 4,
			}

			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			require.True(t, madeProgress)

			sent, _ := topPort.RetrieveOutgoing()
			dr := sent
			require.Equal(t, readFromTop.ID, dr.RspTo)
			require.Equal(t, dataReady.Payload.(memprotocol.DataReadyRsp).Data, dr.Payload.(memprotocol.DataReadyRsp).Data)

			updatedState := &translator.State
			require.Len(t, updatedState.InflightReqToBottom, 1)
		})

		t.Run("should respond write done", func(t *testing.T) {
			setup1(t)
			setup4(t)

			done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           sim.NewID(),
				RspTo:        writeToBottom.ID,
				TrafficBytes: 4,
			}

			bottomPort.Deliver(done)

			madeProgress := tRespondPipeMW.respond()

			require.True(t, madeProgress)

			sent, _ := topPort.RetrieveOutgoing()
			doneMsg := sent
			require.Equal(t, writeFromTop.ID, doneMsg.RspTo)

			updatedState := &translator.State
			require.Len(t, updatedState.InflightReqToBottom, 1)
		})

		t.Run("should stall if TopPort is busy", func(t *testing.T) {
			setup1(t)
			setup4(t)

			dataReady := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
				ID:           sim.NewID(),
				RspTo:        readToBottom.ID,
				TrafficBytes: 4,
			}

			// Fill the top port's outgoing buffer so Send fails.
			fillOutgoing(t, topPort, topBufSize)
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			require.False(t, madeProgress)
			updatedState := &translator.State
			require.Len(t, updatedState.InflightReqToBottom, 2)
		})
	})

	t.Run("state serialization", func(t *testing.T) {
		t.Run("should pass ValidateState", func(t *testing.T) {
			setup1(t)

			err := modeling.ValidateState(state{})
			require.NoError(t, err)
		})
	})

	t.Run("when handling control messages", func(t *testing.T) {
		var (
			readFromTop   messaging.Msg
			writeFromTop  messaging.Msg
			readToBottom  messaging.Msg
			writeToBottom messaging.Msg
			flushReq      messaging.Msg
			restartReq    messaging.Msg
		)

		setup5 := func(t *testing.T) {
			t.Helper()

			readFromTop = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x10040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			readToBottom = messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x20040,
				AccessByteSize: 4},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			writeFromTop = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			writeToBottom = messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x10040},
				ID: sim.NewID(),

				TrafficBytes: 12,
			}

			flushReq = messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdFlush,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          ctrlPort.AsRemote(),
				TrafficBytes: 4,
			}

			restartReq = messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdReset,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          ctrlPort.AsRemote(),
				TrafficBytes: 4,
			}

			nextState := &translator.State
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
		}

		t.Run("rejects Flush as unsupported", func(t *testing.T) {
			setup1(t)
			setup5(t)

			ctrlPort.Deliver(flushReq)

			madeProgress := modelingtest.Tick(translator)

			require.True(t, madeProgress)
			rspMsg, _ := ctrlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, rspMsg)
			rsp := rspMsg
			require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, memcontrolprotocol.ErrUnsupported, rsp.Payload.(memcontrolprotocol.Rsp).Error)
		})

		t.Run("clears in-flight state on Reset", func(t *testing.T) {
			setup1(t)
			setup5(t)

			ctrlPort.Deliver(restartReq)

			madeProgress := modelingtest.Tick(translator)

			require.True(t, madeProgress)
			rspMsg, _ := ctrlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, rspMsg)
			rsp := rspMsg
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			updatedState := &translator.State
			require.Equal(t, controlStateEnabled, updatedState.ControlState)
			require.Empty(t, updatedState.InflightReqToBottom)
		})
	})
}

// fillOutgoing fills a port's outgoing buffer with dummy messages so the next
// CanSend returns false. Each dummy's Src equals the port (required by Send's
// validation) and is sent to a distinct destination.
func fillOutgoing(t *testing.T, p messaging.Port, n int) {
	t.Helper()

	for i := 0; i < n; i++ {
		dummy := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:  p.Owner().(interface{ NewID() uint64 }).NewID(),
			Src: p.AsRemote(),
			Dst: messaging.RemotePort("Dummy"),
		}

		require.True(t, p.CanSend())
		p.Send(dummy)
	}
}
