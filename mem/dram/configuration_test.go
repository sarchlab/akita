package dram

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolStringChoices(t *testing.T) {
	for _, protocol := range []Protocol{
		ProtocolDDR3, ProtocolDDR4, ProtocolDDR5, ProtocolGDDR5, ProtocolGDDR5X, ProtocolGDDR6,
		ProtocolLPDDR, ProtocolLPDDR3, ProtocolLPDDR4, ProtocolLPDDR5,
		ProtocolHBM, ProtocolHBM2, ProtocolHBM3, ProtocolHBM3E, ProtocolHMC,
	} {
		spec := DefaultSpec()
		spec.Protocol = protocol
		require.NotPanics(t, spec.mustBeSupported)
		data, err := json.Marshal(spec)
		require.NoError(t, err)
		var restored Spec
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, spec, restored)
	}
}

func TestRejectUnknownChoicesAtInitialize(t *testing.T) {
	for _, modify := range []func(*Spec){
		func(s *Spec) { s.Protocol = "unknown" }, func(s *Spec) { s.PagePolicy = "unknown" },
	} {
		c := &Comp{}
		c.Spec = DefaultSpec()
		modify(&c.Spec)
		c.Resources.Storage = nil
		expected := "dram: unsupported protocol unknown"
		if c.Spec.PagePolicy == "unknown" {
			expected = "dram: unsupported page policy unknown"
		}
		require.PanicsWithValue(t, expected, func() { c.Middlewares.Initialize(c) })
	}
}
