package zerodefaults

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	Count   int
	Label   string
	Enabled bool
	Ratio   float32
}

var Definition = modeling.ComponentDef[Spec]{Name: "ZeroDefaults"}
