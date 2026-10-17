package missingcomp

type Spec struct {
	N int `json:"n"`
}

func DefaultSpec() Spec { return Spec{N: 1} }
