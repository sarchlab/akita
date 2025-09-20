//go:build ignore
// +build ignore

package rule4_2_missing_sim

type Builder struct {
	spec Spec
}

func MakeBuilder() Builder { return Builder{spec: defaults()} }

// WithSpec configures the builder spec.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithSimulation is intentionally missing to trigger Rule 4.2.

func (b Builder) Build(name string) *Comp {
	if err := b.spec.validate(); err != nil {
		panic(err)
	}
	return &Comp{}
}
