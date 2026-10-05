# packetization — Flit Primitives for Interconnects

Package `packetization` provides packetization primitives for the Akita
simulation framework. Within the `noc` networking stack, messages do not travel
across the network whole: endpoints split each message into fixed-size flits,
the smallest unit that moves between switch ports, and reassemble the flits back
into a message at the receiving endpoint. This package defines the flit type
shared by the `endpoint` and `switches` packages.

## Key Types

### Flit

A `Flit` is a value payload representing one transfer unit on the network.
It travels inside a `messaging.Msg` envelope.

```go
type Flit struct {
    SeqID        int               // index of this flit within its message
    NumFlitInMsg int               // total flits the message was split into
    Msg          messaging.Msg // metadata of the carried message
}
```

- The outer `messaging.Msg` carries the flit's own routing info (`Src`/`Dst`),
  which describes the current hop between an endpoint and a switch port — not
  the final endpoints.
- `Msg` carries the original message's metadata (true `Src`/`Dst`, traffic
  class, byte size), which the receiving endpoint uses to rebuild the message.
- `SeqID` and `NumFlitInMsg` let the receiving endpoint know when every flit of
  a message has arrived and reassembly can complete.

## How It Works

An `endpoint` computes how many flits a message needs from its `TrafficBytes`,
flit byte size, and encoding overhead, then emits that many `Flit` values with a
shared `Msg` payload and increasing `SeqID`. Switches forward each flit
independently using its hop-level `Dst`; the destination endpoint counts
arriving flits per message ID and, once `NumFlitInMsg` flits are in, delivers
the reassembled message to the device port.

The flit's `Msg` field can preserve a complete nested message through JSON.
For the current traffic-only switching network, the sending endpoint explicitly
clears that inner message's `Payload` before packetization. The receiving endpoint
delivers the routing fields with an empty `AssembledMsg` payload. Delivering the
original application payload is a separate change tracked by #495.
