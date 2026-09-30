package countnotint

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	NumOut float64 `json:"num_out" akita:"derived"`
}

var Definition = modeling.ComponentDef[Spec]{Name: "C", PortGroups: []modeling.PortGroupDef{{Name: "Out", CountField: "num_out"}}}
