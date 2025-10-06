//go:build ignore
// +build ignore

package rule2_1_channel

type state struct {
	Notify chan struct{}
}
