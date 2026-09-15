package duplicatejson

import "github.com/sarchlab/akita/v5/modeling"

type Spec struct {
	N int
	B int `json:"N"`
}

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "C"})
