# Internal NoC endpoint

Applications assemble networks with `networkconnector`, `mesh`, `pcie`, or
`nvlink`. This internal package implements their pass-through endpoint.

Build with `MakeBuilder().WithSimulation(s).WithSpec(DefaultSpec()).Build(name)`.
Bind `NetworkPort` and both ends of its point-to-point connection, then append
owned `*twowaybuffered.Port` device ports through `DeviceConnection(ep).BindPort`.
Device binding order is arbitration order. Initialize derives the peer from the
bound connection and freezes further binding. No predicted addresses are needed.

The builder registers a topology resource that checks the device list/order and
network peer on checkpoint restore. These values no longer belong to Spec.
The device connection lives in Resources; outgoing/incoming mutable queues remain
in State. Rebuild old checkpoints after this migration.

Outgoing middleware splits device messages into flits. Incoming middleware
reassembles and delivers the original message. Sources and destinations are
preserved, without an additional device-facing buffer or link hop. Defaults are
a 32-byte flit, 0.25 encoding overhead, and one input/output channel.
