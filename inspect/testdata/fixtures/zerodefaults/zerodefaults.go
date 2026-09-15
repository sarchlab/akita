package zerodefaults

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	Count   int
	Label   string
	Enabled bool
	Ratio   float32
	Array   [2]int
	Slice   []int
	Map     map[int]int
}

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "ZeroDefaults"})
