// Package mesh assembles a rectangular mesh during setup.
package mesh

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
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
	name                    string
	simulation              timing.Simulation
	connector               *networkconnector.Connector
	freq                    timing.Freq
	switchLatency, flitSize int
	linkTransferPerCycle    float64
	gridSize                [3]int
	tiles                   map[[3]int]*tile
	links                   map[[2]int]bool
}

type Builder struct {
	simulation              timing.Simulation
	freq                    timing.Freq
	switchLatency, flitSize int
	linkTransferPerCycle    float64
}

func MakeBuilder() Builder {
	return Builder{freq: timing.GHz, flitSize: 16, linkTransferPerCycle: 1}
}
func (b Builder) WithSimulation(s timing.Simulation) Builder { b.simulation = s; return b }
func (b Builder) WithFreq(f timing.Freq) Builder             { b.freq = f; return b }
func (b Builder) WithSwitchLatency(cycles int) Builder       { b.switchLatency = cycles; return b }
func (b Builder) WithBandwidth(transfersPerCycle float64) Builder {
	b.linkTransferPerCycle = transfersPerCycle
	return b
}
func (b Builder) WithFlitSize(size int) Builder { b.flitSize = size; return b }
func (b Builder) Build(name string) *Connector {
	if b.linkTransferPerCycle <= 0 {
		panic("mesh: bandwidth must be positive")
	}
	c := &Connector{name: name, simulation: b.simulation, freq: b.freq, switchLatency: b.switchLatency,
		flitSize: b.flitSize, linkTransferPerCycle: b.linkTransferPerCycle,
		tiles: make(map[[3]int]*tile), links: make(map[[2]int]bool)}
	c.connector = networkconnector.MakeBuilder().WithSimulation(b.simulation).WithFreq(b.freq).
		WithFlitSize(b.flitSize).WithRouter(meshRouter{c: c}).Build(name)
	return c
}

// AddTile binds devices and creates any newly required transit tiles and links.
func (c *Connector) AddTile(loc [3]int, ports []messaging.Port) {
	c.simulation.RequireSetup()
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
		t := &tile{location: at, switchID: c.connector.AddSwitchWithName(switchName),
			switchName: c.name + "." + switchName, endpointName: endpointName}
		c.tiles[at] = t
		c.connector.ConnectDeviceWithEPName(endpointName, t.switchID, nil, c.deviceParameter())
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
	c.connector.ConnectDeviceWithEPName(t.endpointName, t.switchID, ports, c.deviceParameter())
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
func (c *Connector) deviceParameter() networkconnector.DeviceToSwitchLinkParameter {
	width := int(math.Ceil(c.linkTransferPerCycle))
	return networkconnector.DeviceToSwitchLinkParameter{
		DeviceEndParam: networkconnector.LinkEndDeviceParameter{IncomingBufSize: width,
			OutgoingBufSize:  width,
			NumInputChannel:  width,
			NumOutputChannel: width},
		SwitchEndParam: networkconnector.LinkEndSwitchParameter{IncomingBufSize: width,
			OutgoingBufSize:  width,
			NumInputChannel:  width,
			NumOutputChannel: width,
			Latency:          1},
		LinkParam: networkconnector.LinkParameter{IsIdeal: true, Frequency: c.freq},
	}
}
func (c *Connector) switchParameter() networkconnector.SwitchToSwitchLinkParameter {
	width := int(math.Ceil(c.linkTransferPerCycle))
	end := networkconnector.LinkEndSwitchParameter{IncomingBufSize: width,
		OutgoingBufSize:  width,
		NumInputChannel:  width,
		NumOutputChannel: width,
		Latency:          c.switchLatency}
	return networkconnector.SwitchToSwitchLinkParameter{LeftEndParam: end, RightEndParam: end,
		LinkParam: networkconnector.LinkParameter{IsIdeal: true,
			Frequency:     c.freq * timing.Freq(c.linkTransferPerCycle),
			NumStage:      1,
			CyclePerStage: 1,
			PipelineWidth: 1}}
}
