package messaging

import "encoding/json"

// Msg carries routing information and a registered protocol payload. Payloads
// are values. Slice and map storage is shared between sender and receiver;
// callers must not mutate a message or its payload after Send or Deliver.
// A nil Payload represents metadata-only traffic.
type Msg struct {
	ID           uint64     `json:"ID"`
	Src          RemotePort `json:"Src"`
	Dst          RemotePort `json:"Dst"`
	TrafficClass string     `json:"TrafficClass"`
	TrafficBytes int        `json:"TrafficBytes"`
	RspTo        uint64     `json:"RspTo"`
	Payload      any        `json:"-"`
}

// IsRsp reports whether the message responds to another message.
func (m Msg) IsRsp() bool { return m.RspTo != 0 }

// MarshalJSON preserves the concrete payload type with its registered tag.
func (m Msg) MarshalJSON() ([]byte, error) {
	type plain Msg
	routing, err := json.Marshal(plain(m))
	if err != nil {
		return nil, err
	}
	if m.Payload == nil {
		return routing, nil
	}
	payload, err := msgCodec.Encode(m.Payload)
	if err != nil {
		return nil, err
	}
	// Both encodings are JSON objects. The codec owns the payload's wire fields.
	routing[len(routing)-1] = ','
	return append(routing, payload[1:]...), nil
}

// UnmarshalJSON restores the registered value type, including nested messages.
func (m *Msg) UnmarshalJSON(data []byte) error {
	type plain Msg
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	payload, err := msgCodec.Decode(data)
	if err != nil {
		return err
	}
	decoded.Payload = payload
	*m = Msg(decoded)
	return nil
}
