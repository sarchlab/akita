//go:build ignore
// +build ignore

package rule2_1_nested

type counters struct {
	Reads   uint64
	Writes  uint64
	Latency []int
}

type state struct {
	Mode   int
	Active bool
	Stats  counters
}
