package modeling_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

func TestDomainName(t *testing.T) {
	d := modeling.NewDomain("GPU[0]")

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

	modeling.NewDomain("invalid_name")
}

func TestDomainExposesPorts(t *testing.T) {
	d := modeling.NewDomain("GPU")
	port := messaging.NewPort(nil, 1, 1, "GPU.Driver.ToGPU")

	d.DeclarePort("Top")
	d.AssignPort("Top", port)

	if d.GetPortByName("Top") != port {
		t.Error("expected GetPortByName to return the assigned port")
	}

	ports := d.Ports()
	if len(ports) != 1 || ports[0] != port {
		t.Error("expected Ports to list the assigned port")
	}
}

func TestDomainNesting(t *testing.T) {
	gpu := modeling.NewDomain("GPU[0]")
	sa := modeling.NewDomain("GPU[0].SA[1]")
	port := messaging.NewPort(nil, 1, 1, "GPU[0].SA[1].L1Cache.Top")

	sa.DeclarePort("Top")
	sa.AssignPort("Top", port)

	gpu.DeclarePort("Mem")
	gpu.AssignPort("Mem", sa.GetPortByName("Top"))

	if gpu.GetPortByName("Mem") != port {
		t.Error("expected the nested domain's port to be exposed by the outer domain")
	}
}
