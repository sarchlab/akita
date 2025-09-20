//go:build ignore
// +build ignore

package rule3_1_missing_spec

type Comp struct{}

func (c *Comp) Tick() bool { return false }
