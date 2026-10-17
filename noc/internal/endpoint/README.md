# Internal NoC endpoint

Applications assemble networks with `networkconnector`, `mesh`, `pcie`, or
`nvlink`. This internal package implements their pass-through endpoint.

Build with `MakeBuilder().WithSimulation(s).WithSpec(DefaultSpec()).WithPeer(peer).Build(name)`.
Bind `NetworkPort` and both ends of its point-to-point connection, then append
owned `*twowaybuffered.Port` device ports through `DeviceConnection(ep).BindPort`.
Device binding order is arbitration order. Initialize reads the bound address of
the peer supplied in Resources. No predicted addresses or connection port-discovery
API is needed.

The network assembler owns one topology checkpoint record covering device order
and peer wiring. The endpoint builder does not register a separate record. Direct
endpoint networks register their record explicitly; see `acceptance/one_to_one`.
The device connection lives in Resources; outgoing/incoming mutable queues remain
in State. Rebuild old checkpoints after this migration.

Outgoing middleware splits device messages into flits. Incoming middleware
reassembles and delivers the original message. Sources and destinations are
preserved, without an additional device-facing buffer or link hop. Defaults are
a 32-byte flit, 0.25 encoding overhead, and one input/output channel.
