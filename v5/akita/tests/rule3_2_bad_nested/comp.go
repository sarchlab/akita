//go:build ignore
// +build ignore

package rule3_4_bad_nested

type Comp struct{}

func (c *Comp) Tick() bool { return false }
