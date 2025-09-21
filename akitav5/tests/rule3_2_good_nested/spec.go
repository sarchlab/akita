//go:build ignore
// +build ignore

package rule3_4_good_nested

import "fmt"

type NestedSpec struct {
	Kind   string
	Params map[string]uint64
}

type Spec struct {
	Width   int
	Nested  NestedSpec
	Options []int
}

func (s Spec) validate() error {
	if s.Width <= 0 {
		return fmt.Errorf("width must be > 0")
	}
	return nil
}

func defaults() Spec {
	return Spec{Width: 1, Nested: NestedSpec{Kind: "identity", Params: map[string]uint64{}}}
}
