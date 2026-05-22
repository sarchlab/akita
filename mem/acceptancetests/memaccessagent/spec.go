package memaccessagent

import (
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains the immutable configuration for the MemAccessAgent.
type Spec struct {
	Freq              timing.Freq `json:"freq"`
	MaxAddress        uint64      `json:"max_address"`
	UseVirtualAddress bool        `json:"use_virtual_address"`
}
