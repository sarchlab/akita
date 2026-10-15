# Internal NoC endpoint

Applications assemble networks with `networkconnector`, `mesh`, `pcie`, or
`nvlink`. This internal package implements the pass-through endpoint shared by
those connectors; acceptance tests can also wire endpoints directly.

Start with `Definition.DefaultSpec`, set `Spec.Devices` to the ordered remote
addresses of the device ports, and build with
`MakeBuilder().WithSimulation(s).WithSpec(spec).Build(name)`. Bind the network
port with `ep.BindPort("NetworkPort", networkPort)`, then bind each device with
`DeviceConnection(ep).BindPort(port)` in the declared order. Initialize validates
the list before creating middleware and freezes further binding.

The device connection owns the bound-port list; Resources contains only the
connection reference. Devices must be owned `*twowaybuffered.Port` values and
must not already have a connection. Each rejected BindPort leaves that port and
the connection unchanged. Device membership and order participate in the Spec
hash, so a checkpoint cannot resume with different routing or arbitration order.

Outgoing middleware drains device buffers in order and splits messages into
flits. Incoming middleware reassembles the original message and delivers it to
the matching device port. Forwarding preserves the original Src and Dst and
adds no device-facing link or buffer hop. Default settings are a 32-byte flit,
0.25 encoding overhead, and one input and output channel.
