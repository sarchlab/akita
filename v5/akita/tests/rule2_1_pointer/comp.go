//go:build ignore
// +build ignore

package rule2_1_pointer

type Comp struct{}

func (c *Comp) Tick() bool { return false }
