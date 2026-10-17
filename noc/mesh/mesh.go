// Package mesh provides a connector implementation for the mesh.
package mesh

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/noc/networkconnector"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type tile struct {
	ports []messaging.Port
	sw    int
	rt    *meshRoutingTable
}

// A Connector can help establishing a mesh or torus network.
type Connector struct {
	routePorts []routePort

	connector *networkconnector.Connector

	freq                 timing.Freq
	switchLatency        int
	flitSize             int
	linkTransferPerCycle float64

	gridSize [3]int
	gridCap  [3]int
	grid     [][][]tile
	dstTable map[messaging.RemotePort]*tile
}

// Builder configures an independent connector; With methods do not assemble a network.
type Builder struct {
	simulation           timing.Simulation
	freq                 timing.Freq
	switchLatency        int
	flitSize             int
	linkTransferPerCycle float64
}

func MakeBuilder() Builder { return Builder{freq: timing.GHz, flitSize: 16, linkTransferPerCycle: 1} }

func (b Builder) WithSimulation(sim timing.Simulation) Builder { b.simulation = sim; return b }

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq

	return b
}

func (b Builder) WithSwitchLatency(numCycles int) Builder {
	b.switchLatency = numCycles
	return b
}

func (b Builder) WithBandwidth(transferPerCycle float64) Builder {
	b.linkTransferPerCycle = transferPerCycle
	return b
}

func (b Builder) WithFlitSize(size int) Builder {
	b.flitSize = size

	return b
}

// Build computes derived parameters and begins a new network.
func (b Builder) Build(name string) *Connector {
	if b.linkTransferPerCycle <= 0 {
		panic("mesh: bandwidth must be positive")
	}
	c := &Connector{
		freq: b.freq, switchLatency: b.switchLatency, flitSize: b.flitSize,
		linkTransferPerCycle: b.linkTransferPerCycle,
		dstTable:             make(map[messaging.RemotePort]*tile), gridCap: [3]int{8, 8, 2},
		connector: networkconnector.MakeBuilder().WithSimulation(b.simulation).
			WithFreq(b.freq).WithFlitSize(b.flitSize).Build(name)}
	c.grid = c.initializeGrid(c.gridCap)
	return c
}

// AddTile adds some ports to a give coordinate in the mesh or torus. The
// connector supports 3 dimensional meshes. If a 2D mesh is designed, simply
// set the third coordinate to 0. We assume first first tile always has a
// coordinate of [0,0,0]. Negative coordinate is not supported.
func (c *Connector) AddTile(loc [3]int, ports []messaging.Port) {
	if loc[0] < 0 || loc[1] < 0 || loc[2] < 0 {
		panic("coordinate is negative")
	}

	c.resizeGridToHold(loc)
	c.updateSize(loc)

	c.mergePorts(loc, ports)

	for _, port := range ports {
		c.dstTable[port.AsRemote()] = &c.grid[loc[0]][loc[1]][loc[2]]
	}
}

func (c *Connector) mergePorts(loc [3]int, ports []messaging.Port) {
	if len(c.grid[loc[0]][loc[1]][loc[2]].ports) != 0 {
		fmt.Printf("Tile [%d, %d, %d] already configured. Merging Ports.\n",
			loc[0], loc[1], loc[2])
	}

	c.grid[loc[0]][loc[1]][loc[2]].ports = append(
		c.grid[loc[0]][loc[1]][loc[2]].ports, ports...)
}

func (c *Connector) resizeGridToHold(loc [3]int) {
	resizeNeeded := false

	newGridCap := [3]int{c.gridCap[0], c.gridCap[1], c.gridCap[2]}
	if loc[0] >= c.gridCap[0] {
		newGridCap[0] = loc[0] * 2
		resizeNeeded = true
	}

	if loc[1] >= c.gridCap[1] {
		newGridCap[1] = loc[1] * 2
		resizeNeeded = true
	}

	if loc[2] >= c.gridCap[2] {
		newGridCap[2] = loc[2] * 2
		resizeNeeded = true
	}

	if !resizeNeeded {
		return
	}

	newGrid := c.initializeGrid(newGridCap)

	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				newGrid[x][y][z] = c.grid[x][y][z]
			}
		}
	}

	c.grid = newGrid
	c.gridCap = newGridCap
}

func (c *Connector) updateSize(loc [3]int) {
	if loc[0] >= c.gridSize[0] {
		c.gridSize[0] = loc[0] + 1
	}

	if loc[1] >= c.gridSize[1] {
		c.gridSize[1] = loc[1] + 1
	}

	if loc[2] >= c.gridSize[2] {
		c.gridSize[2] = loc[2] + 1
	}
}

func (c *Connector) initializeGrid(cap [3]int) [][][]tile {
	grid := make([][][]tile, cap[0])
	for x := 0; x < cap[0]; x++ {
		grid[x] = make([][]tile, cap[1])
		for y := 0; y < cap[1]; y++ {
			grid[x][y] = make([]tile, cap[2])
			for z := 0; z < cap[2]; z++ {
				grid[x][y][z] = tile{
					rt: &meshRoutingTable{
						x: x,
						y: y,
						z: z,
					},
				}
			}
		}
	}

	return grid
}

type routePort struct {
	dst  *messaging.RemotePort
	port messaging.Port
}

// EstablishNetwork creates the switches, links, and the routing tables for the
// network to built.
func (c *Connector) EstablishNetwork() {
	c.createSwitches()
	c.createLinks()
	for _, r := range c.routePorts {
		*r.dst = c.connector.PortAddress(r.port)
	}
	c.routePorts = nil
	c.connector.BuildSwitches()
}

func (c *Connector) createLinks() {
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				c.connectWithFrontSwitch(x, y, z)
				c.connectWithTopSwitch(x, y, z)
				c.connectWithLeftSwitch(x, y, z)
			}
		}
	}
}

func (c *Connector) createSwitches() {
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				swName := fmt.Sprintf("SW[%d][%d][%d]", x, y, z)
				rt := &meshRoutingTable{
					x:        x,
					y:        y,
					z:        z,
					dstTable: c.dstTable,
				}
				sw := c.connector.AddSwitchWithNameAndRoutingTable(swName, rt)

				c.grid[x][y][z].rt = rt
				c.grid[x][y][z].sw = sw

				transferPerCycle := int(math.Ceil(c.linkTransferPerCycle))
				epName := fmt.Sprintf("EP[%d][%d][%d]", x, y, z)
				_, swPort := c.connector.ConnectDeviceWithEPName(
					epName,
					sw, c.grid[x][y][z].ports,
					networkconnector.DeviceToSwitchLinkParameter{
						DeviceEndParam: networkconnector.LinkEndDeviceParameter{
							IncomingBufSize:  transferPerCycle,
							OutgoingBufSize:  transferPerCycle,
							NumInputChannel:  transferPerCycle,
							NumOutputChannel: transferPerCycle,
						},
						SwitchEndParam: networkconnector.LinkEndSwitchParameter{
							IncomingBufSize:  transferPerCycle,
							OutgoingBufSize:  transferPerCycle,
							Latency:          1,
							NumInputChannel:  transferPerCycle,
							NumOutputChannel: transferPerCycle,
						},
						LinkParam: networkconnector.LinkParameter{
							IsIdeal:       true,
							Frequency:     c.freq,
							NumStage:      0,
							CyclePerStage: 0,
							PipelineWidth: 0,
						},
					})

				c.routePorts = append(c.routePorts, routePort{&rt.local, swPort})
			}
		}
	}
}

func (c *Connector) connectWithLeftSwitch(x, y, z int) {
	if x == 0 {
		return
	}

	x1 := x - 1

	curr := c.grid[x][y][z]
	left := c.grid[x1][y][z]

	portA, portB := c.createLink(left.sw, curr.sw, "Right", "Left")
	c.routePorts = append(c.routePorts, routePort{&left.rt.right, portA})
	c.routePorts = append(c.routePorts, routePort{&curr.rt.left, portB})
}

func (c *Connector) connectWithTopSwitch(x, y, z int) {
	if y == 0 {
		return
	}

	y1 := y - 1

	curr := c.grid[x][y][z]
	top := c.grid[x][y1][z]

	portA, portB := c.createLink(top.sw, curr.sw, "Bottom", "Top")
	c.routePorts = append(c.routePorts, routePort{&top.rt.bottom, portA})
	c.routePorts = append(c.routePorts, routePort{&curr.rt.top, portB})
}

func (c *Connector) connectWithFrontSwitch(x, y, z int) {
	if z == 0 {
		return
	}

	z1 := z - 1

	curr := c.grid[x][y][z]
	front := c.grid[x][y][z1]

	portA, portB := c.createLink(front.sw, curr.sw, "Back", "Front")
	c.routePorts = append(c.routePorts, routePort{&front.rt.back, portA})
	c.routePorts = append(c.routePorts, routePort{&curr.rt.front, portB})
}

func (c *Connector) createLink(
	a, b int,
	DirectionA, DirectionB string,
) (portA, portB messaging.Port) {
	transferPerCycle := int(math.Ceil(c.linkTransferPerCycle))

	return c.connector.ConnectSwitches(a, b,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  transferPerCycle,
				OutgoingBufSize:  transferPerCycle,
				Latency:          c.switchLatency,
				NumInputChannel:  transferPerCycle,
				NumOutputChannel: transferPerCycle,
				PortName:         DirectionA,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  transferPerCycle,
				OutgoingBufSize:  transferPerCycle,
				Latency:          c.switchLatency,
				NumInputChannel:  transferPerCycle,
				NumOutputChannel: transferPerCycle,
				PortName:         DirectionB,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     c.freq * timing.Freq(c.linkTransferPerCycle),
				NumStage:      1,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}
