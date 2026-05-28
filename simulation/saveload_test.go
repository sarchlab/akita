package simulation

import (
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type manifestSpec struct {
	Value int `json:"value"`
}

type manifestState struct {
	Counter int `json:"counter"`
}

type manifestStorageComponent struct {
	*modeling.Component[manifestSpec, manifestState]
	storage     *mem.Storage
	storageName string
}

func (c *manifestStorageComponent) Resources() []Resource {
	return []Resource{
		mem.NewStorageCheckpointResource(c.storageName, c.storage),
	}
}

var _ = Describe("Save/Load", func() {
	var s *Simulation

	BeforeEach(func() {
		s = MakeBuilder().WithoutMonitoring().Build()
	})

	AfterEach(func() {
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	})

	Context("verifyQuiescence", func() {
		It("should pass when all ports empty", func() {
			port := testPort{name: "p1"}

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(Succeed())
		})

		It("should fail when incoming buffer not empty", func() {
			port := testPort{name: "p1", incoming: 1}

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(
				MatchError(ContainSubstring("incoming")))
		})

		It("should fail when outgoing buffer not empty", func() {
			port := testPort{name: "p1", outgoing: 2}

			s.ports = append(s.ports, port)
			Expect(s.verifyQuiescence()).To(
				MatchError(ContainSubstring("outgoing")))
		})
	})

	Context("Save", func() {
		It("should fail if not quiescent", func() {
			port := testPort{name: "busy", incoming: 1}

			s.ports = append(s.ports, port)
			err := s.Save(GinkgoT().TempDir())
			Expect(err).To(MatchError(ContainSubstring("cannot save")))
		})

		It("should save metadata when quiescent", func() {
			timing.ResetIDGenerator()
			timing.UseSequentialIDGenerator()

			// Generate a few IDs to advance the counter.
			timing.GetIDGenerator().Generate()
			timing.GetIDGenerator().Generate()

			dir := GinkgoT().TempDir()
			err := s.Save(dir)
			Expect(err).To(Succeed())

			// Metadata should exist.
			_, err = os.Stat(dir + "/metadata.json")
			Expect(err).To(Succeed())

			// Components and resources dirs should exist.
			_, err = os.Stat(dir + "/components")
			Expect(err).To(Succeed())
			_, err = os.Stat(dir + "/resources")
			Expect(err).To(Succeed())
		})

		It("should save a strict manifest with components and resources", func() {
			timing.ResetIDGenerator()
			timing.UseSequentialIDGenerator()

			comp := modeling.NewBuilder[manifestSpec, manifestState]().
				WithEngine(s.GetEngine()).
				WithFreq(1 * timing.GHz).
				WithSpec(manifestSpec{Value: 7}).
				Build("ManifestComp")
			comp.State = manifestState{Counter: 3}

			storageComp := &manifestStorageComponent{
				Component: modeling.NewBuilder[manifestSpec, manifestState]().
					WithEngine(s.GetEngine()).
					WithFreq(1 * timing.GHz).
					WithSpec(manifestSpec{Value: 9}).
					Build("StorageComp"),
				storage:     mem.NewStorage(4 * mem.KB),
				storageName: "Program.Memory",
			}

			s.RegisterComponent(comp)
			s.RegisterComponent(storageComp)

			dir := GinkgoT().TempDir()
			err := s.Save(dir)
			Expect(err).To(Succeed())

			data, err := os.ReadFile(dir + "/manifest.json")
			Expect(err).To(Succeed())

			var manifest checkpointManifest
			err = json.Unmarshal(data, &manifest)
			Expect(err).To(Succeed())

			Expect(manifest.Version).To(Equal(checkpointManifestVersion))
			Expect(manifest.Components).To(HaveKey("ManifestComp"))
			Expect(manifest.Components).To(HaveKey("StorageComp"))
			Expect(manifest.Resources).To(HaveKey("Program.Memory"))
			Expect(manifest.Resources["Program.Memory"].Kind).To(Equal("mem.Storage"))
			Expect(manifest.Resources["Program.Memory"].Path).
				To(Equal("resources/Program.Memory.bin"))
		})
	})

	Context("Load", func() {
		It("should restore metadata", func() {
			timing.ResetIDGenerator()
			timing.UseSequentialIDGenerator()

			// Advance ID generator.
			timing.GetIDGenerator().Generate()
			timing.GetIDGenerator().Generate()
			timing.GetIDGenerator().Generate()
			savedNextID := timing.GetIDGeneratorNextID()

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
			timing.SetIDGeneratorNextID(999)

			err = s2.Load(dir)
			Expect(err).To(Succeed())

			Expect(timing.GetIDGeneratorNextID()).To(Equal(savedNextID))
		})

		It("should fail when a manifest component file is missing", func() {
			timing.ResetIDGenerator()
			timing.UseSequentialIDGenerator()

			comp := modeling.NewBuilder[manifestSpec, manifestState]().
				WithEngine(s.GetEngine()).
				WithFreq(1 * timing.GHz).
				WithSpec(manifestSpec{Value: 7}).
				Build("StrictComp")
			s.RegisterComponent(comp)

			dir := GinkgoT().TempDir()
			err := s.Save(dir)
			Expect(err).To(Succeed())
			Expect(os.Remove(dir + "/components/StrictComp.json")).To(Succeed())

			s2 := MakeBuilder().WithoutMonitoring().Build()
			defer func() {
				s2.Terminate()
				os.Remove("akita_sim_" + s2.ID() + ".sqlite3")
			}()

			comp2 := modeling.NewBuilder[manifestSpec, manifestState]().
				WithEngine(s2.GetEngine()).
				WithFreq(1 * timing.GHz).
				WithSpec(manifestSpec{Value: 7}).
				Build("StrictComp")
			s2.RegisterComponent(comp2)

			err = s2.Load(dir)
			Expect(err).To(MatchError(ContainSubstring("open component file StrictComp")))
		})
	})
})
