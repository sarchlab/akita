package negativebounds

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct{ N int }

var Definition = modeling.ComponentDef[Spec]{Name: "C", PortGroups: []modeling.PortGroupDef{{Name: "P", MinCount: -1}}}
