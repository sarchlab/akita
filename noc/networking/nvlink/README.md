# nvlink — NVLink, PCIe, and Ethernet Multi-Fabric Interconnect

Package `nvlink` provides a connector that builds a multi-device network for the
Akita simulation framework, combining PCIe, NVLink, and Ethernet links. Within
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
- **NVLink** links directly join two devices' NVLink switches; bandwidth comes
  from the NVLink version, and `numLink` sets the link's pipeline width.
- **Ethernet** switches and links connect nodes over a higher-latency,
  non-ideal fabric.

Routing uses a `BandwidthFirstRouter`, so paths prefer higher-bandwidth links
(e.g. NVLink between peers) over PCIe. Routing tables are populated by
`EstablishRoute`.

## Key Types

### Connector

```go
type Connector struct { /* ... */ }

func NewConnector() *Connector
func (c *Connector) CreateNetwork(name string)
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int)
func (c *Connector) AddPCIeSwitch() (switchID int)
func (c *Connector) ConnectSwitchesWithPCIeLink(switchAID, switchBID int)
func (c *Connector) PlugInDevice(pcieSwitchID int, devicePorts []messaging.Port) (deviceID int)
func (c *Connector) ConnectDevicesWithNVLink(deviceA, deviceB, numLink int)
func (c *Connector) CreateEthernetSwitch() (switchID int)
func (c *Connector) ConnectSwitchesWithEthernetLink(switchAID, switchBID int)
func (c *Connector) EstablishRoute()
```

`PlugInDevice` returns a `deviceID` used by `ConnectDevicesWithNVLink` to join
two devices. Switch IDs returned by the `Add*`/`Create*` calls are used to wire
PCIe and Ethernet links between switches.

## Builder Pattern

`NewConnector` returns a `Connector` with chained `WithX` options. Defaults are
`1 GHz`, PCIe Gen4 x16, NVLink v2, PCIe/NVLink switch latency 140, Ethernet
switch latency 100000, and Ethernet bandwidth 1.25 GiB/s.

```go
connector := nvlink.NewConnector().
    WithEngine(engine).
    WithFrequency(1 * timing.GHz).
    WithPCIeVersion(4, 16).
    WithPCIeSwitchLatency(140).
    WithNVLinkVersion(2).
    WithNVLinkSwitchLatency(140).
    WithEthernetBandwidth(1.25 * (1 << 30)).
    WithEthernetSwitchLatency(100000).
    WithMonitor(monitor)
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFrequency(f)` | Frequency of the network components |
| `WithPCIeVersion(v, w)` / `WithPCIeBandwidth(b)` | PCIe link bandwidth |
| `WithPCIeSwitchLatency(n)` | Cycles per PCIe switch hop |
| `WithNVLinkVersion(v)` / `WithNVLinkBandwidth(b)` | NVLink link bandwidth |
| `WithNVLinkSwitchLatency(n)` | Cycles per NVLink switch hop |
| `WithEthernetBandwidth(b)` | Ethernet link bandwidth |
| `WithEthernetSwitchLatency(n)` | Cycles per Ethernet switch hop |
| `WithMonitor(m)` | Monitor for inspecting component state |
| `WithVisTracer(t)` | Tracer for visualizing network tasks |

## Usage

```go
connector := nvlink.NewConnector().
    WithEngine(engine).
    WithFrequency(1 * timing.GHz).
    WithPCIeVersion(4, 16).
    WithNVLinkVersion(2)

connector.CreateNetwork("NVLink")

root := connector.AddRootComplex([]messaging.Port{cpu.GetPortByName("PCIe")})

dev0 := connector.PlugInDevice(root, []messaging.Port{gpu0.GetPortByName("Net")})
dev1 := connector.PlugInDevice(root, []messaging.Port{gpu1.GetPortByName("Net")})

connector.ConnectDevicesWithNVLink(dev0, dev1, 4) // 4 NVLinks between the GPUs

connector.EstablishRoute()
```

After `EstablishRoute`, host-device traffic flows over PCIe while device-device
traffic prefers the higher-bandwidth NVLink path.
