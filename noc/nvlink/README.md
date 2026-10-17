# nvlink — NVLink and PCIe Multi-Fabric Interconnect

Package `nvlink` provides a connector that builds a multi-device network for the
Akita simulation framework, combining PCIe and NVLink links. Within
`noc`, it constructs the switches and links that move messages between a CPU and
a set of devices (e.g. GPUs) that talk over PCIe to the host and high-bandwidth
NVLink directly to one another.

## How It Works

A `Connector` models each device as a pair of switches: a device switch (on the
PCIe side) and an NVLink switch (for peer-to-peer links). Plugging in a device
creates both switches, links them, attaches the device ports to the device
switch, and connects the device switch to a PCIe switch toward the host.

- **PCIe** links carry CPU-device traffic; their flit size is derived from the
  PCIe bandwidth (version × lane width) and the network frequency.
- **NVLink** links directly join two devices' NVLink switches. The connector
  computes link parameters from the NVLink version and `numLink`, but the
  underlying ideal-link implementation ignores their frequency and width.
  `WithNVLinkBandwidth`, `WithNVLinkVersion`, and `numLink` do not currently
  change link throughput.

All links are ideal direct connections at the default network frequency;
non-ideal latency and per-link bandwidth are not modeled. Although routing
uses a `BandwidthFirstRouter`, every link reports infinite bandwidth, so it
cannot prefer NVLink based on bandwidth. `simulation.Initialize()` populates reachable
paths without that guarantee. These limitations are tracked in
[#502](https://github.com/sarchlab/akita/issues/502).

## Key Types

### Connector

```go
type Connector struct { /* ... */ }

func MakeBuilder() Builder
func (b Builder) Build(name string) *Connector
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int)
func (c *Connector) AddPCIeSwitch() (switchID int)
func (c *Connector) ConnectSwitchesWithPCIeLink(switchAID, switchBID int)
func (c *Connector) PlugInDevice(pcieSwitchID int, devicePorts []messaging.Port) (deviceID int)
func (c *Connector) ConnectDevicesWithNVLink(deviceA, deviceB, numLink int)
```

`PlugInDevice` returns a `deviceID` used by `ConnectDevicesWithNVLink` to join
two devices. Switch IDs returned by the `Add*` calls are used to wire
PCIe links between switches.

## Builder Pattern

`MakeBuilder` returns a value builder with chained `WithX` options. Defaults are
`1 GHz`, PCIe Gen4 x16, NVLink v2, and PCIe/NVLink switch latency 140.

```go
connector := nvlink.MakeBuilder().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    WithPCIeVersion(4, 16).
    WithPCIeSwitchLatency(140).
    WithNVLinkVersion(2).
    WithNVLinkSwitchLatency(140).
    Build("NVLink")
```

### Builder Options

| Method | Description |
|---|---|
| `WithSimulation(sim)` | Simulation context (required) |
| `WithFreq(f)` | Frequency of the network components |
| `WithPCIeVersion(v, w)` / `WithPCIeBandwidth(b)` | Sets flit size from PCIe bandwidth and network frequency; not a per-link rate limit |
| `WithPCIeSwitchLatency(n)` | Cycles per PCIe switch hop |
| `WithNVLinkVersion(v)` / `WithNVLinkBandwidth(b)` | Stored parameters; currently no effect on link throughput |
| `WithNVLinkSwitchLatency(n)` | Cycles per NVLink switch hop |

The connector builds its switches and endpoints with `WithSimulation(sim)`,
so the simulation's tracer and monitor see each of them; the connector has
no tracer or monitor option of its own.

## Usage

```go
connector := nvlink.MakeBuilder().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    WithPCIeVersion(4, 16).
    WithNVLinkVersion(2).
    Build("NVLink")

root := connector.AddRootComplex([]messaging.Port{cpu.Ports.PCIe})

dev0 := connector.PlugInDevice(root, []messaging.Port{gpu0.Ports.Net})
dev1 := connector.PlugInDevice(root, []messaging.Port{gpu1.Ports.Net})

connector.ConnectDevicesWithNVLink(dev0, dev1, 4) // numLink currently has no timing effect

if err := sim.Initialize(); err != nil { panic(err) }
```

After `simulation.Initialize()`, the network can carry host-device and device-device
traffic through the assembled topology. Do not use this connector to compare
PCIe and NVLink bandwidth until the link-model limitations above are resolved.

All `WithX` methods belong to the value `Builder`. `Build(name)` returns a fresh
`*Connector`; a builder can be reused for independent networks. Derived values
are computed by `Build`, so setting frequency and bandwidth in either order
produces the same configuration. Use the connector's assembly methods after
Build, then call `simulation.Initialize()` after all wiring is complete.
