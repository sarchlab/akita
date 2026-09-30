package modeling_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

type saPorts struct {
	Top messaging.Port
}

type gpuPorts struct {
	Mem messaging.Port
}

func TestDomainName(t *testing.T) {
	port := messaging.NewPort(nil, 1, 1, "GPU[0].L2.Bottom")
	d := modeling.NewDomain("GPU[0]", gpuPorts{Mem: port})

	if d.Name() != "GPU[0]" {
		t.Errorf("expected name %q, got %q", "GPU[0]", d.Name())
	}
}

func TestDomainNameMustBeValid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected NewDomain to panic on an invalid name")
		}
	}()

	modeling.NewDomain("invalid_name",
		gpuPorts{Mem: messaging.NewPort(nil, 1, 1, "A.B")})
}

func TestDomainNeedsEveryPort(t *testing.T) {
	expectPanic(t, "port Mem is not given", func() {
		modeling.NewDomain("GPU", gpuPorts{})
	})
}

func TestDomainNesting(t *testing.T) {
	port := messaging.NewPort(nil, 1, 1, "GPU[0].SA[1].L1Cache.Top")
	sa := modeling.NewDomain("GPU[0].SA[1]", saPorts{Top: port})
	gpu := modeling.NewDomain("GPU[0]", gpuPorts{Mem: sa.Ports.Top})

	if gpu.Ports.Mem != port {
		t.Error("expected the nested domain's port to be exposed by the outer domain")
	}
}
