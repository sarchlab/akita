//go:generate mockgen -destination=mock_sim_test.go -package=gmmu_test -write_package_comment=false github.com/sarchlab/akita/v4/sim Engine,Port
//go:generate mockgen -destination=mock_vm_test.go -package=gmmu_test -write_package_comment=false github.com/sarchlab/akita/v4/mem/vm PageTable
//go:generate mockgen -destination=mock_mem_test.go -package=gmmu_test -write_package_comment=false github.com/sarchlab/akita/v4/mem/mem AddressToPortMapper

package gmmu
