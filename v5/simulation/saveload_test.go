package simulation

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Save/Load", func() {
	var (
		mockCtrl *gomock.Controller
		s        *Simulation
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		s = MakeBuilder().WithoutMonitoring().Build()
	})

	AfterEach(func() {
		mockCtrl.Finish()
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	})

	Context("verifyQuiescence", func() {
		It("should pass when all ports empty", func() {
			port := NewMockPort(mockCtrl)
			port.EXPECT().Name().Return("p1").AnyTimes()
			port.EXPECT().NumIncoming().Return(0).AnyTimes()
			port.EXPECT().NumOutgoing().Return(0).AnyTimes()

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(Succeed())
		})

		It("should fail when incoming buffer not empty", func() {
			port := NewMockPort(mockCtrl)
			port.EXPECT().Name().Return("p1").AnyTimes()
			port.EXPECT().NumIncoming().Return(1).AnyTimes()
			port.EXPECT().NumOutgoing().Return(0).AnyTimes()

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(
				MatchError(ContainSubstring("incoming")))
		})

		It("should fail when outgoing buffer not empty", func() {
			port := NewMockPort(mockCtrl)
			port.EXPECT().Name().Return("p1").AnyTimes()
			port.EXPECT().NumIncoming().Return(0).AnyTimes()
			port.EXPECT().NumOutgoing().Return(2).AnyTimes()

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(
				MatchError(ContainSubstring("outgoing")))
		})
	})

	Context("Save", func() {
		It("should fail if not quiescent", func() {
			port := NewMockPort(mockCtrl)
			port.EXPECT().Name().Return("busy").AnyTimes()
			port.EXPECT().NumIncoming().Return(1).AnyTimes()
			port.EXPECT().NumOutgoing().Return(0).AnyTimes()

			s.ports = append(s.ports, port)
			err := s.Save(GinkgoT().TempDir())
			Expect(err).To(MatchError(ContainSubstring("cannot save")))
		})

		It("should save metadata when quiescent", func() {
			sim.ResetIDGenerator()
			sim.UseSequentialIDGenerator()

			// Generate a few IDs to advance the counter.
			sim.GetIDGenerator().Generate()
			sim.GetIDGenerator().Generate()

			dir := GinkgoT().TempDir()
			err := s.Save(dir)
			Expect(err).To(Succeed())

			// Metadata should exist.
			_, err = os.Stat(dir + "/metadata.json")
			Expect(err).To(Succeed())

			// Components and storage dirs should exist.
			_, err = os.Stat(dir + "/components")
			Expect(err).To(Succeed())
			_, err = os.Stat(dir + "/storage")
			Expect(err).To(Succeed())
		})
	})

	Context("Load", func() {
		It("should restore metadata", func() {
			sim.ResetIDGenerator()
			sim.UseSequentialIDGenerator()

			// Advance ID generator.
			sim.GetIDGenerator().Generate()
			sim.GetIDGenerator().Generate()
			sim.GetIDGenerator().Generate()
			savedNextID := sim.GetIDGeneratorNextID()

			dir := GinkgoT().TempDir()
			err := s.Save(dir)
			Expect(err).To(Succeed())

			// Create a new simulation with same structure.
			s2 := MakeBuilder().WithoutMonitoring().Build()
			defer func() {
				s2.Terminate()
				os.Remove("akita_sim_" + s2.ID() + ".sqlite3")
			}()

			// Reset ID to a different value.
			sim.SetIDGeneratorNextID(999)

			err = s2.Load(dir)
			Expect(err).To(Succeed())

			Expect(sim.GetIDGeneratorNextID()).To(Equal(savedNextID))
		})
	})
})
