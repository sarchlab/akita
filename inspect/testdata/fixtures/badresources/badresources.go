package badresources

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct{ N int }

type Builder struct{ n int }

func (b Builder) WithResources(n int) Builder {
	b.n = n
	return b
}

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
