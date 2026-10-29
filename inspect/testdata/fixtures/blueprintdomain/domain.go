package blueprintdomain

import (
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type Spec struct {
	Count int
	Peer  messaging.RemotePort
}

func DefaultSpec() Spec { return Spec{Count: 4, Peer: ""} }

type Ports struct {
	Top      messaging.Port
	Children []messaging.Port
}

func Node(spec Spec) *blueprint.DomainNode { return blueprint.NewDomain[Spec, Ports](spec, build) }
func build(_ Spec, _ *blueprint.Blueprint) { panic("inspection must never execute this") }
