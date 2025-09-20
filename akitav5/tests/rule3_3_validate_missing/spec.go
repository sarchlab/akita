//go:build ignore
// +build ignore

package rule3_3_validate_missing

func defaults() Spec {
	return Spec{Width: 1}
}

type Spec struct {
	Width int
}
