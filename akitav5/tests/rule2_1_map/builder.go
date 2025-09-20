//go:build ignore
// +build ignore

package rule2_1_map

type Builder struct {
	Engine any
	Freq   int
}

func MakeBuilder() Builder { return Builder{} }

func (b Builder) WithEngine(e any) Builder { b.Engine = e; return b }

func (b Builder) WithFreq(f int) Builder { b.Freq = f; return b }

func (b Builder) Build(name string) *Comp { return &Comp{} }
