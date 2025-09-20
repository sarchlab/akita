//go:build ignore
// +build ignore

package rule3_4_bad_nested

import "fmt"

type NestedSpec struct {
	Pointer *int
}

type Spec struct {
	Nested NestedSpec
}

func (s Spec) validate() error {
	return fmt.Errorf("always fail")
}

func defaults() Spec {
	return Spec{}
}
