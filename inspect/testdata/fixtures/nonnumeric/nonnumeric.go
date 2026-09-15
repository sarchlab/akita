package nonnumeric

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	N string `akita:"min=1"`
}

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "C"})
