//go:build ignore
// +build ignore

package rule3_2_defaults_missing

import "fmt"

type Spec struct {
	Width int
}

func (s Spec) validate() error {
	if s.Width <= 0 {
		return fmt.Errorf("width must be > 0")
	}
	return nil
}
