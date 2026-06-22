# packetization — Flit Primitives for Interconnects

Package `packetization` provides packetization primitives for the Akita
simulation framework. Within the `noc` networking stack, messages do not travel
across the network whole: endpoints split each message into fixed-size flits,
the smallest unit that moves between switch ports, and reassemble the flits back
into the original message at the receiving endpoint. This package defines the
flit type shared by the `endpoint` and `switches` packages.

## Key Types

### Flit

A `Flit` is a concrete network message (it embeds `messaging.MsgMeta`, so it
satisfies the `messaging.Msg` contract) that represents one transfer unit on the
network.

```go
type Flit struct {
    messaging.MsgMeta
    SeqID        int               // index of this flit within its message
    NumFlitInMsg int               // total flits the message was split into
    Msg          messaging.MsgMeta // metadata of the carried message
    Payload      messaging.Msg     // the carried message itself (final flit only)
}
```

- The embedded `MsgMeta` carries the flit's own routing info (`Src`/`Dst`),
  which describes the current hop between an endpoint and a switch port — not
  the final endpoints.
- `Msg` carries the original message's metadata (true `Src`/`Dst`, traffic
  class, byte size). Every flit carries it, so each switch can route the flit
  independently and the receiving endpoint can group flits by message ID.
- `Payload` carries the original concrete message. It rides on the **final**
  flit (and is `nil` on the others), so the message is carried exactly once. The
  receiving endpoint delivers this concrete message — not a metadata-only
  stand-in — so payload-bearing protocols (PCIe `mem.WriteReq`, kernel-launch
  requests, RDMA data, …) survive the network crossing intact.
- `SeqID` and `NumFlitInMsg` let the receiving endpoint know when every flit of
  a message has arrived and reassembly can complete.

Because `Payload` is a polymorphic `messaging.Msg`, `Flit` implements custom
`MarshalJSON`/`UnmarshalJSON` that encode the payload through the message codec
(the same machinery that checkpoints in-flight messages in port buffers), so a
flit held in a port buffer or in switch State round-trips across a checkpoint
with its concrete payload type preserved.

## How It Works

An `endpoint` computes how many flits a message needs from its `TrafficBytes`,
flit byte size, and encoding overhead, then emits that many `Flit` values with a
shared `Msg` metadata and increasing `SeqID`, attaching the original message to
the last flit as its `Payload`. Switches forward each flit independently using
its hop-level `Dst`; the destination endpoint counts arriving flits per message
ID and, once `NumFlitInMsg` flits are in, delivers the carried `Payload` message
to the device port. The flit count still models traffic and timing, while the
payload makes the delivered message the real thing rather than a metadata
envelope.
