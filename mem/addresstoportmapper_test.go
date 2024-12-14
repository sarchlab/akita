package mem

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

var _ = Describe("InterleavedAddressToPortMapper", func() {
	var (
		addressToPortMapper *InterleavedAddressPortMapper
	)

	BeforeEach(func() {
		addressToPortMapper = new(InterleavedAddressPortMapper)
		addressToPortMapper.UseAddressSpaceLimitation = true
		addressToPortMapper.LowAddress = 0
		addressToPortMapper.HighAddress = 4 * GB
		addressToPortMapper.InterleavingSize = 4096
		addressToPortMapper.LowModules = make([]modeling.RemotePort, 0)
		for i := 0; i < 6; i++ {
			addressToPortMapper.LowModules = append(
				addressToPortMapper.LowModules,
				modeling.RemotePort(fmt.Sprintf("LowModule[%d].Port", i)),
			)
		}
		addressToPortMapper.ModuleForOtherAddresses =
			modeling.RemotePort("LowModuleOther.Port")
	})

	It("should find low module if address is in-space", func() {
		Expect(addressToPortMapper.Find(0)).To(
			BeIdenticalTo(addressToPortMapper.LowModules[0]))
		Expect(addressToPortMapper.Find(4096)).To(
			BeIdenticalTo(addressToPortMapper.LowModules[1]))
		Expect(addressToPortMapper.Find(4097)).To(
			BeIdenticalTo(addressToPortMapper.LowModules[1]))
	})

	It("should use a special module for all the addresses that does not fall "+
		"in range", func() {
		Expect(addressToPortMapper.Find(4 * GB)).To(
			BeIdenticalTo(addressToPortMapper.ModuleForOtherAddresses))
	})
})
