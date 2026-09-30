package pointerspec

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct{ P *int }

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
