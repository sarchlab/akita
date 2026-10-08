package direct

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// NewConnection creates an ideal direct connection with the given tick frequency.
// It registers the connection and its event handler with s. Ports are attached
// separately with PlugIn. The simulation must be non-nil and the name valid.
func NewConnection(name string, s timing.Simulation, freq timing.Freq) *Connection {
	if s == nil {
		panic("direct: simulation is required")
	}

	naming.MustBeValid(name)
	spec := configuration{Freq: freq}
	modeling.MustBeCheckpointable[configuration, State](name, spec)

	conn := &Connection{
		name: name,
		spec: spec,
		// Secondary ticks run after components in the same cycle.
		ticks: ticking.NewSecondaryScheduler(name, s, freq),
		ports: ports{portMap: make(map[messaging.RemotePort]int)},
	}
	s.Engine().RegisterHandler(name, conn)
	s.RegisterConnection(conn)

	return conn
}
