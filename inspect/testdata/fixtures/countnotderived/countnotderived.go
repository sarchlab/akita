package countnotderived

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	NumOut int `json:"num_out"`
}

var Definition = modeling.ComponentDef[Spec]{Name: "C", PortGroups: []modeling.PortGroupDef{{Name: "Out", CountField: "num_out"}}}
