# routing — Network Routing Tables

Package `routing` provides routing tables for the Akita simulation framework.
Within the `noc` networking stack, switches consult a `routing.Table` to decide
which output port a flit should take toward its final destination. The
higher-level mesh, PCIe, and NVLink builders populate these tables (via the
`networkconnector`) so that individual switches only need to perform a simple
table lookup at run time.

## Key Types

### Table

`Table` is a concrete setup-time table populated by routers. Routes are computed
at Initialize; changing a table afterwards is not supported. Destinations
and ports are identified by `messaging.RemotePort` (a string port name).

```go
type Table struct { /* private route storage */ }
func NewTable() *Table
func (t *Table) FindPort(dst messaging.RemotePort) messaging.RemotePort
func (t *Table) DefineRoute(finalDst, outputPort messaging.RemotePort)
func (t *Table) DefineDefaultRoute(outputPort messaging.RemotePort)
```

- `DefineRoute` records that traffic whose final destination is `finalDst`
  should leave through `outputPort` (the next hop).
- `DefineDefaultRoute` sets the port used when no explicit route matches.
- `FindPort` returns the next-hop port for `dst`, falling back to the default
  port if `dst` was never defined.

## How It Works

`NewTable` returns a table backed by a `map` from final destination
to next-hop output port plus an optional default port.

```go
table := routing.NewTable()
table.DefineRoute("Device2.Port", "Switch0.Port1")
table.DefineDefaultRoute("Switch0.Port0")

next := table.FindPort("Device2.Port") // "Switch0.Port1"
```

A lookup that misses the map returns the default port, which is the empty
string until `DefineDefaultRoute` is called. Routes are normally established in
bulk by a `networkconnector.Router` (such as the Floyd–Warshall router) rather
than defined by hand.
