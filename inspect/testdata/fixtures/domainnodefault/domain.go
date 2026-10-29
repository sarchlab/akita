package domainnodefault

import "github.com/sarchlab/akita/v5/blueprint"

type Spec struct{}
type Ports struct{}

func Node(spec Spec) *blueprint.DomainNode { return blueprint.NewDomain[Spec, Ports](spec, build) }
func build(_ Spec, _ *blueprint.Blueprint) {}
