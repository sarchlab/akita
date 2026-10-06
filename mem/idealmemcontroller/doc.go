// Package idealmemcontroller provides an implementation of an ideal memory
// controller, which has a fixed latency and unlimited concurrency.
//
// The controller is a ticking component. The system builder builds it with
// Definition.Builder(), supplying the backing storage in Resources (required)
// and the Top and Control port instances, created with messaging.NewPort, with
// WithPorts.
package idealmemcontroller

//go:generate mockgen -destination mock_sim_test.go -package idealmemcontroller -write_package_comment=false github.com/sarchlab/akita/v5/sim/messaging Port
//go:generate mockgen -destination mock_timing_test.go -package idealmemcontroller -write_package_comment=false github.com/sarchlab/akita/v5/sim/timing Engine
