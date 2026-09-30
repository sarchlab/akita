package dashcontainer

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	N    int   `json:"n"`
	List []int `json:"-"`
}

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
