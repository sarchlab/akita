package networkconnector

import (
	"fmt"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestConnector(t *testing.T) {
	t.Run("should establish route in a simple network", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()
		connector := MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).Build("Network")

		connector.AddSwitch(SwitchConfig{})

		for i := 0; i < 2; i++ {
			port := twowaybuffered.MakeBuilder().WithSimulation(sim).Build()
			port.BindOwner(routeTestOwner{}, fmt.Sprintf("Device[%d].Port", i))
			connector.ConnectDevice(0, []messaging.Port{port},
				DeviceConfig{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
				})
		}
	})

	t.Run("should establish route in a small tree", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()
		connector := MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).Build("Network")

		for i := 0; i < 3; i++ {
			connector.AddSwitch(SwitchConfig{})
		}

		for i := 0; i < 2; i++ {
			port := twowaybuffered.MakeBuilder().WithSimulation(sim).Build()
			port.BindOwner(routeTestOwner{}, fmt.Sprintf("Device[%d].Port", i))
			connector.ConnectDevice(1+i, []messaging.Port{port},
				DeviceConfig{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
				})
		}

		for i := 1; i < 3; i++ {
			connector.ConnectSwitches(i, (i-1)/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
				})
		}
	})

	t.Run("should establish route in a large tree", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()
		connector := MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).Build("Network")

		for i := 0; i < 16; i++ {
			connector.AddSwitch(SwitchConfig{})
		}

		for i := 0; i < 8; i++ {
			port := twowaybuffered.MakeBuilder().WithSimulation(sim).Build()
			port.BindOwner(routeTestOwner{}, fmt.Sprintf("Device[%d].Port", i))
			connector.ConnectDevice(8+i, []messaging.Port{port},
				DeviceConfig{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
				})
		}

		for i := 1; i < 16; i++ {
			connector.ConnectSwitches(i, i/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{RouteBufferCapacity: 1, ForwardBufferCapacity: 1, SendBufferCapacity: 1,
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
				})
		}
	})
}

type routeTestOwner struct{}

func (routeTestOwner) NotifyRecv(messaging.Port)     {}
func (routeTestOwner) NotifyPortFree(messaging.Port) {}

func (o routeTestOwner) Name() string                           { return "RouteTestOwner" }
func (o routeTestOwner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
