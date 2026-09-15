package timing

import (
	"testing"
)

func BenchmarkIDGeneratorNewID(b *testing.B) {
	var ids IDGenerator
	b.ReportAllocs()
	for b.Loop() {
		ids.NewID()
	}
}
