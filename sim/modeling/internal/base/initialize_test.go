package base

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type wrongInitializer struct{}

func (*wrongInitializer) Initialize() {}

type valueInitializer struct{}

func (valueInitializer) Initialize(*int) {}

func TestRejectIncorrectInitializerSignatures(t *testing.T) {
	c := 0
	require.Panics(t, func() { InitializeElement(&wrongInitializer{}, &c) })
	require.Panics(t, func() { InitializeElement(&valueInitializer{}, &c) })
}
