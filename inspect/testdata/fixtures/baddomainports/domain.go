package baddomainports

import "github.com/sarchlab/akita/v5/blueprint"

type Spec struct{}

func DefaultSpec() Spec { return Spec{} }

type Ports struct{ Top int }

func Node(spec Spec) *blueprint.DomainNode { return blueprint.NewDomain[Spec, Ports](spec, build) }
func build(_ Spec, _ *blueprint.Blueprint) {}
