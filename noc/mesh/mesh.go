// Package mesh assembles a rectangular mesh during setup.
package mesh

import (
	"fmt"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type tile struct {
	location                 [3]int
	switchID                 int
	switchName, endpointName string
	ports                    []messaging.Port
}

// Connector grows a rectangular mesh from the origin to the largest added
// coordinates, including empty transit tiles. Repeated AddTile calls append
// device ports in arbitration order.
type Connector struct {
	inputWidth       int
	outputWidth      int
	incomingCapacity int
	outgoingCapacity int
	routeCapacity    int
	forwardCapacity  int
	sendCapacity     int

	name          string
	simulation    modeling.Simulation
	connector     *networkconnector.Connector
	switchLatency int

	gridSize [3]int
	tiles    map[[3]int]*tile
	links    map[[2]int]bool
}

// Builder configures a rectangular mesh connector.
type Builder struct {
	inputWidth       int
	outputWidth      int
	incomingCapacity int
	outgoingCapacity int
	routeCapacity    int
	forwardCapacity  int
	sendCapacity     int

	simulation              modeling.Simulation
	freq                    timing.Freq
	switchLatency, flitSize int
}

// MakeBuilder returns the default direct configuration.
func MakeBuilder() Builder {
	return Builder{
		inputWidth:       1,
		outputWidth:      1,
		incomingCapacity: 1,
		outgoingCapacity: 1,
		routeCapacity:    1,
		forwardCapacity:  1,
		sendCapacity:     1,
		freq:             timing.GHz,
		flitSize:         16,
	}
}

// WithSimulation sets the owning simulation.
func (b Builder) WithSimulation(s modeling.Simulation) Builder { b.simulation = s; return b }

// WithFreq sets the switch frequency.
func (b Builder) WithFreq(f timing.Freq) Builder { b.freq = f; return b }

// WithSwitchLatency sets the switch pipeline latency in cycles.
func (b Builder) WithSwitchLatency(cycles int) Builder { b.switchLatency = cycles; return b }

// WithFlitSize sets the number of bytes per flit.
func (b Builder) WithFlitSize(size int) Builder { b.flitSize = size; return b }

// WithInputWidth sets the flits per cycle.
func (b Builder) WithInputWidth(n int) Builder { b.inputWidth = n; return b }

// WithOutputWidth sets the flits per cycle.
func (b Builder) WithOutputWidth(n int) Builder { b.outputWidth = n; return b }

// WithIncomingBufferCapacity sets the queue capacity in flits.
func (b Builder) WithIncomingBufferCapacity(n int) Builder { b.incomingCapacity = n; return b }

// WithOutgoingBufferCapacity sets the queue capacity in flits.
func (b Builder) WithOutgoingBufferCapacity(n int) Builder { b.outgoingCapacity = n; return b }

// WithRouteBufferCapacity sets the queue capacity in flits.
func (b Builder) WithRouteBufferCapacity(n int) Builder { b.routeCapacity = n; return b }

// WithForwardBufferCapacity sets the queue capacity in flits.
func (b Builder) WithForwardBufferCapacity(n int) Builder { b.forwardCapacity = n; return b }

// WithSendBufferCapacity sets the queue capacity in flits.
func (b Builder) WithSendBufferCapacity(n int) Builder { b.sendCapacity = n; return b }

// Build creates an empty connector ready for AddTile calls.
func (b Builder) Build(name string) *Connector {
	if b.inputWidth <= 0 ||
		b.outputWidth <= 0 ||
		b.incomingCapacity <= 0 ||
		b.outgoingCapacity <= 0 ||
		b.routeCapacity <= 0 ||
		b.forwardCapacity <= 0 ||
		b.sendCapacity <= 0 {
		panic("mesh: widths and buffer capacities must be positive")
	}
	if b.switchLatency < 0 {
		panic("mesh: switch latency cannot be negative")
	}

	c := &Connector{
		inputWidth:       b.inputWidth,
		outputWidth:      b.outputWidth,
		incomingCapacity: b.incomingCapacity,
		outgoingCapacity: b.outgoingCapacity,
		routeCapacity:    b.routeCapacity,
		forwardCapacity:  b.forwardCapacity,
		sendCapacity:     b.sendCapacity,
		name:             name,
		simulation:       b.simulation,
		switchLatency:    b.switchLatency,
		tiles:            make(map[[3]int]*tile), links: make(map[[2]int]bool)}
	c.connector = networkconnector.MakeBuilder().WithSimulation(b.simulation).WithFreq(b.freq).
		WithFlitSize(b.flitSize).WithRouter(meshRouter{c: c}).Build(name)
	return c
}

// AddTile binds devices and creates any newly required transit tiles and links.
func (c *Connector) AddTile(loc [3]int, ports []messaging.Port) {
	assembly.RequireSetup(c.simulation)
	for _, coordinate := range loc {
		if coordinate < 0 {
			panic("coordinate is negative")
		}
	}
	for i, coordinate := range loc {
		c.gridSize[i] = max(c.gridSize[i], coordinate+1)
	}
	c.eachLocation(func(at [3]int) {
		if c.tiles[at] != nil {
			return
		}
		switchName := fmt.Sprintf("SW[%d][%d][%d]", at[0], at[1], at[2])
		endpointName := fmt.Sprintf("EP[%d][%d][%d]", at[0], at[1], at[2])
		t := &tile{location: at, switchID: c.connector.AddSwitch(networkconnector.SwitchConfig{Name: switchName}),
			switchName: c.name + "." + switchName, endpointName: endpointName}
		c.tiles[at] = t
		c.connector.ConnectDevice(t.switchID, nil, c.deviceParameter(endpointName))
	})
	c.eachLocation(func(at [3]int) {
		// Preserve the old mesh's front, top, left construction order.
		for axis := 2; axis >= 0; axis-- {
			if at[axis] == 0 {
				continue
			}
			before := at
			before[axis]--
			a, b := c.tiles[before], c.tiles[at]
			key := [2]int{a.switchID, b.switchID}
			if c.links[key] {
				continue
			}
			c.connector.ConnectSwitches(a.switchID, b.switchID, c.switchParameter())
			c.links[key] = true
		}
	})
	t := c.tiles[loc]
	c.connector.ConnectDevice(t.switchID, ports, c.deviceParameter(t.endpointName))
	t.ports = append(t.ports, ports...)
}

func (c *Connector) eachLocation(f func([3]int)) {
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				f([3]int{x, y, z})
			}
		}
	}
}
func (c *Connector) deviceParameter(name string) networkconnector.DeviceConfig {
	return networkconnector.DeviceConfig{Name: name,
		DeviceEndParam: c.deviceEnd(),
		SwitchEndParam: c.switchEnd(1),
	}
}
func (c *Connector) switchParameter() networkconnector.SwitchToSwitchLinkParameter {
	end := c.switchEnd(c.switchLatency)
	return networkconnector.SwitchToSwitchLinkParameter{LeftEndParam: end, RightEndParam: end}
}

func (c *Connector) switchEnd(latency int) networkconnector.LinkEndSwitchParameter {
	return networkconnector.LinkEndSwitchParameter{
		IncomingBufSize:       c.incomingCapacity,
		OutgoingBufSize:       c.outgoingCapacity,
		NumInputChannel:       c.inputWidth,
		NumOutputChannel:      c.outputWidth,
		RouteBufferCapacity:   c.routeCapacity,
		ForwardBufferCapacity: c.forwardCapacity,
		SendBufferCapacity:    c.sendCapacity,
		Latency:               latency,
	}
}

func (c *Connector) deviceEnd() networkconnector.LinkEndDeviceParameter {
	return networkconnector.LinkEndDeviceParameter{
		IncomingBufSize:  c.incomingCapacity,
		OutgoingBufSize:  c.outgoingCapacity,
		NumInputChannel:  c.inputWidth,
		NumOutputChannel: c.outputWidth,
	}
}
