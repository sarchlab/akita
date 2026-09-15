package nonfinite

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	N int `akita:"min=NaN"`
}

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "C"})
