package networkconnector

import (
	"fmt"
	"testing"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

//go:generate mockgen -destination "mock_simulation_test.go" -self_package=github.com/sarchlab/akita/v4/noc/networking/networkconnector -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/simulation Simulation
//go:generate mockgen -destination "mock_timing_test.go" -self_package=github.com/sarchlab/akita/v4/noc/networking/networkconnector -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine

func TestNetworkconnector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networkconnector Suite")
}

var _ = Describe("Connector", func() {
	var (
		mockCtrl *gomock.Controller
		engine   timing.Engine
		sim      *MockSimulation
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(engine).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should establish route in a simple network", func() {
		connector := MakeConnector().
			WithSimulation(sim).
			WithDefaultFreq(1 * timing.GHz)
		connector.NewNetwork("Network")

		connector.AddSwitch()

		for i := 0; i < 2; i++ {
			port := modeling.PortBuilder{}.
				WithSimulation(sim).
				WithIncomingBufCap(1).
				WithOutgoingBufCap(1).
				Build(fmt.Sprintf("Port%d", i))

			connector.ConnectDevice(0, []modeling.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * timing.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})

	It("should establish route in a small tree", func() {
		connector := MakeConnector().
			WithSimulation(sim).
			WithDefaultFreq(1 * timing.GHz)
		connector.NewNetwork("Network")

		for i := 0; i < 3; i++ {
			connector.AddSwitch()
		}

		for i := 0; i < 2; i++ {
			port := modeling.PortBuilder{}.
				WithSimulation(sim).
				WithIncomingBufCap(1).
				WithOutgoingBufCap(1).
				Build(fmt.Sprintf("Port%d", i))

			connector.ConnectDevice(1+i, []modeling.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * timing.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		for i := 1; i < 3; i++ {
			connector.ConnectSwitches(i, (i-1)/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * timing.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})

	It("should establish route in a large tree", func() {
		connector := MakeConnector().
			WithSimulation(sim).
			WithDefaultFreq(1 * timing.GHz)
		connector.NewNetwork("Network")

		for i := 0; i < 16; i++ {
			connector.AddSwitch()
		}

		for i := 0; i < 8; i++ {
			port := modeling.PortBuilder{}.
				WithSimulation(sim).
				WithIncomingBufCap(1).
				WithOutgoingBufCap(1).
				Build(fmt.Sprintf("Port%d", i))

			connector.ConnectDevice(8+i, []modeling.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * timing.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		for i := 1; i < 16; i++ {
			connector.ConnectSwitches(i, i/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * timing.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})
})
