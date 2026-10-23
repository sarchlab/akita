package simplebankedmemory

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBankConversionConfiguration(t *testing.T) {
	c := &Comp{}
	c.Spec = DefaultSpec()
	require.NotPanics(t, func() { c.State.Initialize(c) })
	require.Equal(t, uint64(640), bankSelectionAddress(c.Spec, 640))
	c.Spec.BankAddressConversion = BankAddressConversion{
		Kind: ConversionInterleaving, InterleavingSize: 128, NumElements: 4, ElementIndex: 1, Offset: 1024,
	}
	data, err := json.Marshal(c.Spec)
	require.NoError(t, err)
	var restored Spec
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, c.Spec, restored)
	require.NotPanics(t, func() { c.State.Initialize(c) })
	require.Equal(t, uint64(128), bankSelectionAddress(restored, 1664))
	require.Panics(t, func() { bankSelectionAddress(restored, 1024) })
	require.Panics(t, func() { bankSelectionAddress(restored, 512) })
	t.Log("JSON configuration strips controller interleaving: global 1664 -> local 128; foreign addresses rejected")
}

func TestInvalidBankConversionRejectedAtInitialize(t *testing.T) {
	for _, conversion := range []BankAddressConversion{
		{Kind: "unknown"},
		{Kind: ConversionInterleaving, NumElements: 4},
		{Kind: ConversionInterleaving, InterleavingSize: 128},
		{Kind: ConversionInterleaving, InterleavingSize: 128, NumElements: 4, ElementIndex: -1},
		{Kind: ConversionInterleaving, InterleavingSize: 128, NumElements: 4, ElementIndex: 4},
		{Kind: ConversionInterleaving, InterleavingSize: 1 << 63, NumElements: 2},
	} {
		c := &Comp{}
		c.Spec = DefaultSpec()
		c.Spec.BankAddressConversion = conversion
		require.Panics(t, func() { c.State.Initialize(c) })
	}
}
