package analysis

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"

	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
)

type sampleMsg struct {
	meta sim.MsgMeta
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.meta
}

var _ = Describe("Port Analyzer", func() {
	var (
		mockCtrl *gomock.Controller

		port          *MockPort
		incommingPort *MockPort
		outgoingPort  *MockPort
		timeTeller    *MockTimeTeller
		portLogger    *MockPerfLogger

		portAnalyzer *PortAnalyzer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		port = NewMockPort(mockCtrl)
		port.EXPECT().Name().Return("PortName").AnyTimes()

		incommingPort = NewMockPort(mockCtrl)
		incommingPort.EXPECT().Name().Return("IncomingPort").AnyTimes()

		outgoingPort = NewMockPort(mockCtrl)
		outgoingPort.EXPECT().Name().Return("OutgoingPort").AnyTimes()

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
		msg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 100,
				Src:          port,
				Dst:          outgoingPort,
			},
		}

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.1))
		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.1)).AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  1.0,
			Unit:   "Msg",
		})

		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})
	})

	It("should log traffic if only a middle period has value", func() {
		msg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 100,
				Dst:          port,
				Src:          incommingPort,
			},
		}

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(20.5)).Times(2)
		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  20.0,
			End:    21.0,
			Src:    "PortName",
			Linker: "IncomingPort",
			Dir:    "Incoming",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  20.0,
			End:    21.0,
			Src:    "PortName",
			Linker: "IncomingPort",
			Dir:    "Incoming",
			Value:  1.0,
			Unit:   "Msg",
		})

		timeTeller.EXPECT().
			CurrentTime().
			Return(sim.VTimeInSec(26.5)).
			AnyTimes()
		portAnalyzer.summarize()
	})

	It("should log incoming and outgoing traffic", func() {
		outMsg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 100,
				Src:          port,
				Dst:          outgoingPort,
			},
		}
		inMsg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 10000,
				Dst:          port,
				Src:          incommingPort,
			},
		}

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.1)).Times(2)
		portAnalyzer.Func(sim.HookCtx{
			Item:   outMsg,
			Domain: port,
		})
		portAnalyzer.Func(sim.HookCtx{
			Item:   inMsg,
			Domain: port,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.1)).AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  1.0,
			Unit:   "Msg",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "IncomingPort",
			Dir:    "Incoming",
			Value:  10000.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "IncomingPort",
			Dir:    "Incoming",
			Value:  1.0,
			Unit:   "Msg",
		})

		portAnalyzer.Func(sim.HookCtx{
			Item:   outMsg,
			Domain: port,
		})
	})

	It("should log period traffic when there is a gap period", func() {
		msg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 100,
				Src:          port,
				Dst:          outgoingPort,
			},
		}

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.1))
		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3.1)).AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  1.0,
			Unit:   "Msg",
		})

		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})
	})

	It("should log period traffic when simulation ends", func() {
		msg := &sampleMsg{
			meta: sim.MsgMeta{
				TrafficBytes: 100,
				Src:          port,
				Dst:          outgoingPort,
			},
		}

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.1))
		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3.1)).AnyTimes()
		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  0.0,
			End:    1.0,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  1.0,
			Unit:   "Msg",
		})

		portAnalyzer.Func(sim.HookCtx{
			Item:   msg,
			Domain: port,
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  3.0,
			End:    3.1,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  100.0,
			Unit:   "Byte",
		})

		portLogger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:  3.0,
			End:    3.1,
			Src:    "PortName",
			Linker: "OutgoingPort",
			Dir:    "Outgoing",
			Value:  1.0,
			Unit:   "Msg",
		})

		portAnalyzer.summarize()
	})
})
