package simplebankedmemory

import "fmt"

type ConversionKind string

const ConversionInterleaving ConversionKind = "interleaving"

type BankAddressConversion struct {
	Kind             ConversionKind `json:"kind"`
	InterleavingSize uint64         `json:"interleaving_size"`
	NumElements      int            `json:"num_elements"`
	ElementIndex     int            `json:"element_index"`
	Offset           uint64         `json:"offset"`
}

func (c BankAddressConversion) validate() {
	switch c.Kind {
	case "":
		return
	case ConversionInterleaving:
		if c.InterleavingSize == 0 || c.NumElements <= 0 ||
			c.ElementIndex < 0 || c.ElementIndex >= c.NumElements {
			panic("simplebankedmemory: invalid interleaving geometry")
		}
		if c.InterleavingSize > ^uint64(0)/uint64(c.NumElements) {
			panic("simplebankedmemory: interleaving round size overflows uint64")
		}
	default:
		panic(fmt.Sprintf("simplebankedmemory: unsupported conversion kind %q", c.Kind))
	}
}
