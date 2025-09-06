package idealmemcontrollerv5

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestIdealmemcontrollerv5(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "IdealMemControllerV5 Suite")
}

