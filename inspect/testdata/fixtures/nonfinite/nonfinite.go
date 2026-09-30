package nonfinite

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	N int `akita:"min=NaN"`
}

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
