package dram

// controlState is this component's private execution state.
type controlState int

const (
	controlStateEnabled controlState = iota
	controlStatePaused
	controlStateDraining
)
