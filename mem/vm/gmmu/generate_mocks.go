//go:generate mockgen -destination=mock_engine.go -package=gmmu github.com/sarchlab/akita/v4/sim Engine
//go:generate mockgen -destination=mock_port.go -package=gmmu github.com/sarchlab/akita/v4/sim Port
//go:generate mockgen -destination=mock_vm.go -package=gmmu github.com/sarchlab/akita/v4/mem/vm PageTable
//go:generate mockgen -destination=mock_mem.go -package=gmmu github.com/sarchlab/akita/v4/mem/mem AddressToPortMapper

package gmmu
