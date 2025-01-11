package analysis

import (
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/mock/gomock"

	// . "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type sampleMsg struct {
	modeling.MsgMeta
}

func (m sampleMsg) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

func (m sampleMsg) Clone() modeling.Msg {
	newMsg := m
	newMsg.ID = id.Generate()

	return newMsg
}

var _ = Describe("Port Analyzer", func() {
	var (
		mockCtrl *gomock.Controller

		port          *MockPort
		incommingPort *MockPort
		outgoingPort  *MockPort
		timeTeller    *MockTimeTeller
		portLogger    *MockPerfLogger
		portAnalyzer  *PortAnalyzer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		port = NewMockPort(mockCtrl)
		port.EXPECT().Name().Return("PortName").AnyTimes()
		port.EXPECT().AsRemote().
			Return(modeling.RemotePort("PortName")).
			AnyTimes()

		incommingPort = NewMockPort(mockCtrl)
		incommingPort.EXPECT().Name().Return("IncomingPort").AnyTimes()
		incommingPort.EXPECT().AsRemote().
			Return(modeling.RemotePort("IncomingPort")).
			AnyTimes()

		outgoingPort = NewMockPort(mockCtrl)
		outgoingPort.EXPECT().Name().Return("OutgoingPort").AnyTimes()
		outgoingPort.EXPECT().AsRemote().
			Return(modeling.RemotePort("OutgoingPort")).
			AnyTimes()

		timeTeller = NewMockTimeTeller(mockCtrl)
		portLogger = NewMockPerfLogger(mockCtrl)

		portAnalyzer = MakePortAnalyzerBuilder().
			WithPerfLogger(portLogger).
			WithTimeTeller(timeTeller).
			WithPeriod(1).
			WithPort(port).
			Build()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should log period traffic", func() {
		msg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 100,
				Src:          port.AsRemote(),
				Dst:          outgoingPort.AsRemote(),
			},
		}

		timeTeller.EXPECT().Now().Return(timing.VTimeInSec(0.1))
		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(1.1)).
			AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})
	})

	It("should log traffic if only a middle period has value", func() {
		msg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 100,
				Dst:          port.AsRemote(),
				Src:          incommingPort.AsRemote(),
			},
		}

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(20.5)).
			Times(2)
		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       20.0,
			End:         21.0,
			Where:       "PortName",
			WhereRemote: "IncomingPort",
			What:        "Incoming",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       20.0,
			End:         21.0,
			Where:       "PortName",
			WhereRemote: "IncomingPort",
			What:        "Incoming",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(26.5)).
			AnyTimes()
		portAnalyzer.summarize()
	})

	It("should log incoming and outgoing traffic", func() {
		outMsg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 100,
				Src:          port.AsRemote(),
				Dst:          outgoingPort.AsRemote(),
			},
		}
		inMsg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 10000,
				Dst:          port.AsRemote(),
				Src:          incommingPort.AsRemote(),
			},
		}

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(0.1)).
			Times(2)
		portAnalyzer.Func(hooking.HookCtx{
			Item:   outMsg,
			Domain: port,
		})
		portAnalyzer.Func(hooking.HookCtx{
			Item:   inMsg,
			Domain: port,
		})

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(1.1)).
			AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "IncomingPort",
			What:        "Incoming",
			EntryType:   "Traffic",
			Value:       10000.0,
			Unit:        "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "IncomingPort",
			What:        "Incoming",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portAnalyzer.Func(hooking.HookCtx{
			Item:   outMsg,
			Domain: port,
		})
	})

	It("should log period traffic when there is a gap period", func() {
		msg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 100,
				Src:          port.AsRemote(),
				Dst:          outgoingPort.AsRemote(),
			},
		}

		timeTeller.EXPECT().Now().Return(timing.VTimeInSec(0.1))
		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(3.1)).
			AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})
	})

	It("should log period traffic when simulation ends", func() {
		msg := sampleMsg{
			modeling.MsgMeta{
				TrafficBytes: 100,
				Src:          port.AsRemote(),
				Dst:          outgoingPort.AsRemote(),
			},
		}

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(0.1))

		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().
			Now().
			Return(timing.VTimeInSec(3.1)).
			AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       0.0,
			End:         1.0,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portAnalyzer.Func(hooking.HookCtx{
			Item:   msg,
			Domain: port,
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       3.0,
			End:         3.1,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       100.0,
			Unit:        "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:       3.0,
			End:         3.1,
			Where:       "PortName",
			WhereRemote: "OutgoingPort",
			What:        "Outgoing",
			EntryType:   "Traffic",
			Value:       1.0,
			Unit:        "Msg",
		})

		portAnalyzer.summarize()
	})
})
