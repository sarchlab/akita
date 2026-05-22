package datamover

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the data mover.
type Spec struct {
	Freq                   timing.Freq `json:"freq"`
	BufferSize             uint64      `json:"buffer_size"`
	InsideByteGranularity  uint64      `json:"inside_byte_granularity"`
	OutsideByteGranularity uint64      `json:"outside_byte_granularity"`

	InsideMapperKind             string                 `json:"inside_mapper_kind"`
	InsideMapperPorts            []messaging.RemotePort `json:"inside_mapper_ports"`
	InsideMapperInterleavingSize uint64                 `json:"inside_mapper_interleaving_size"`

	OutsideMapperKind             string                 `json:"outside_mapper_kind"`
	OutsideMapperPorts            []messaging.RemotePort `json:"outside_mapper_ports"`
	OutsideMapperInterleavingSize uint64                 `json:"outside_mapper_interleaving_size"`
}
