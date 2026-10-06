# packetization — Flit Primitives for Interconnects

Endpoints split messages into flits for network timing and reassemble them into
complete application messages at the destination. Switches route each flit
independently.

## Flit layout

```go
type Flit struct {
    MsgID        uint64
    Dst          messaging.RemotePort
    SeqID        int
    NumFlitInMsg int
    MsgTaskID    uint64
    Msg          messaging.Msg // complete original message on flit 0 only
}
```

Every flit has a transport header: the original message ID and destination,
its sequence number and total flit count, and the message's tracing task ID.
The outer `messaging.Msg` holds the flit's own ID and current-hop source and
destination. Switches use `Flit.Dst` to select the next hop.

Only flit 0 holds the original message, including all routing metadata and its
registered concrete payload. Other flits have a zero `Msg`, omitted from JSON.
This avoids serializing the application payload once per flit. The existing
`messaging.Msg` codec preserves the nested payload's type without a separate
message wrapper or shared-message registry.

## Reassembly and checkpoints

The receiver groups flits by `MsgID` and records their sequence numbers. Flits
may arrive in any order: flit 0 can arrive before or after the body flits. Its
message is retained in checkpointed reassembly state until every flit arrives.
A nil application payload is valid; `SeqID == 0` identifies the head.

Once complete, the original message waits for capacity on the destination device
port and is delivered unchanged. Flit count and timing still derive from
`TrafficBytes`, `FlitByteSize`, and `EncodingOverhead`; the Go payload is carried
for functional correctness, not converted to physical network bytes.

The transport protocol has only the symmetric `link` role. Device ports receive
the application's own payload, so there is no `AssembledMsg` or `Delivery` role.
