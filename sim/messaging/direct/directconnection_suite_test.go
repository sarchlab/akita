package direct_test

//go:generate mockgen -destination "mock_sim_test.go" -self_package=github.com/sarchlab/akita/v5/sim/messaging/direct -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/messaging Port,Connection
//go:generate mockgen -destination "mock_timing_test.go" -self_package=github.com/sarchlab/akita/v5/sim/messaging/direct -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/timing Engine,Event,Handler
