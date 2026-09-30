package duplicateport

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct{ N int }

var Definition = modeling.ComponentDef[Spec]{Name: "C", Ports: []modeling.PortDef{{Name: "P"}, {Name: "P"}}}
