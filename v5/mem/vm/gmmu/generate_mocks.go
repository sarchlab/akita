//go:generate mockgen -destination=mock_engine.go -package=gmmu github.com/sarchlab/akita/v5/sim Engine
//go:generate mockgen -destination=mock_port_test.go -package=gmmu github.com/sarchlab/akita/v5/sim Port
//go:generate mockgen -destination=mock_vm.go -package=gmmu github.com/sarchlab/akita/v5/mem/vm PageTable
//go:generate mockgen -destination=mock_mem.go -package=gmmu github.com/sarchlab/akita/v5/mem/mem AddressToPortMapper

package gmmu
