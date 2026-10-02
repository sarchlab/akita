package modeling_test

import (
	"fmt"
	"strings"
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
	port := messaging.NewPort("GPU[0].L2.Bottom", 1, 1)
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
		gpuPorts{Mem: messaging.NewPort("A.B", 1, 1)})
}

func TestDomainNeedsEveryPort(t *testing.T) {
	expectPanic(t, "port Mem is not given", func() {
		modeling.NewDomain("GPU", gpuPorts{})
	})
}

func TestDomainNesting(t *testing.T) {
	port := messaging.NewPort("GPU[0].SA[1].L1Cache.Top", 1, 1)
	sa := modeling.NewDomain("GPU[0].SA[1]", saPorts{Top: port})
	gpu := modeling.NewDomain("GPU[0]", gpuPorts{Mem: sa.Ports.Top})

	if gpu.Ports.Mem != port {
		t.Error("expected the nested domain's port to be exposed by the outer domain")
	}
}

func expectPanic(t *testing.T, substr string, f func()) {
	t.Helper()

	defer func() {
		t.Helper()

		r := recover()
		if r == nil {
			t.Fatalf("expected a panic containing %q", substr)
		}
		if !strings.Contains(fmt.Sprint(r), substr) {
			t.Fatalf("panic %q does not contain %q", fmt.Sprint(r), substr)
		}
	}()

	f()
}
