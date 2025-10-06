//go:build ignore
// +build ignore

package rule2_1_map

import "fmt"

type Spec struct {
	Width int
}

func defaults() Spec { return Spec{Width: 1} }

func (s Spec) validate() error {
	if s.Width <= 0 {
		return fmt.Errorf("width must be > 0")
	}
	return nil
}
