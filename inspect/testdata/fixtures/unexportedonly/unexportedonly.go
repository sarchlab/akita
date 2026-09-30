package unexportedonly

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	count int
}

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
