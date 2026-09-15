package negativebounds

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct{ N int }

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "C", PortGroups: []modeling.PortGroupDef{{Name: "P", MinCount: -1}}})
