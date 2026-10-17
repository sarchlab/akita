package switches

import "github.com/sarchlab/akita/v5/sim/messaging"

type testWiring struct {
	links []Link
	table RoutingTable
}

func (w *testWiring) Links() []Link        { return w.links }
func (w *testWiring) Routes() RoutingTable { return w.table }

type fixedRoute messaging.RemotePort

func (r fixedRoute) FindPort(messaging.RemotePort) messaging.RemotePort {
	return messaging.RemotePort(r)
}
