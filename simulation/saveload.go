package simulation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/timing"
)

// StateSaver is implemented by components that can save their state to a
// writer. modeling.Component[S,T] satisfies this via its SaveState method.
type StateSaver interface {
	SaveState(w io.Writer) error
}

// StateLoader is implemented by components that can load their state from a
// reader. modeling.Component[S,T] satisfies this via its LoadState method.
type StateLoader interface {
	LoadState(r io.Reader) error
}

// Resource represents non-timing program state that can be shared across
// components and checkpointed independently from component state.
type Resource interface {
	Name() string
	Kind() string
	Format() string
	FileExtension() string
	Identity() string
	Save(w io.Writer) error
	Load(r io.Reader) error
}

// ResourceOwner is implemented by components that reference resources that
// should be registered with the simulation.
type ResourceOwner interface {
	Resources() []Resource
}

// WakeupResetter is implemented by event-driven components that need to
// reset their pending wakeup guard after loading state from a checkpoint.
type WakeupResetter interface {
	ResetWakeup()
}

// checkpointMetadata is the top-level metadata saved with a checkpoint.
type checkpointMetadata struct {
	EngineTime      timing.VTimeInSec `json:"engine_time"`
	IDGeneratorNext uint64            `json:"id_generator_next"`
}

// Save persists the simulation state to the given directory path.
//
// All port buffers must be empty (quiescence) before saving. The event queue
// is NOT serialized — it will be reconstructed via TickLater on Load.
// Connections are NOT serialized — they are reconstructed from build code.
func (s *Simulation) Save(path string) error {
	if err := s.verifyQuiescence(); err != nil {
		return fmt.Errorf("cannot save: %w", err)
	}

	if err := s.createCheckpointDirs(path); err != nil {
		return err
	}

	if err := s.saveMetadata(path); err != nil {
		return err
	}

	manifest := newCheckpointManifest()

	if err := s.saveComponentStates(path, &manifest); err != nil {
		return err
	}

	if err := s.saveResources(path, &manifest); err != nil {
		return err
	}

	if err := writeCheckpointManifest(path, manifest); err != nil {
		return err
	}

	return nil
}

func (s *Simulation) createCheckpointDirs(path string) error {
	if err := os.MkdirAll(filepath.Join(path, "components"), 0o755); err != nil {
		return fmt.Errorf("create component dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(path, "resources"), 0o755); err != nil {
		return fmt.Errorf("create resources dir: %w", err)
	}

	return nil
}

func (s *Simulation) saveMetadata(path string) error {
	meta := checkpointMetadata{
		EngineTime:      s.engine.CurrentTime(),
		IDGeneratorNext: timing.GetIDGeneratorNextID(),
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(path, "metadata.json"), metaData, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

func (s *Simulation) saveComponentStates(
	root string,
	manifest *checkpointManifest,
) error {
	for _, comp := range s.components {
		saver, ok := comp.(StateSaver)
		if !ok {
			continue
		}

		relPath := checkpointRelPath("components", comp.Name(), ".json")
		filePath := checkpointAbsPath(root, relPath)

		f, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("create component file %s: %w", comp.Name(), err)
		}

		if err := saver.SaveState(f); err != nil {
			f.Close()
			return fmt.Errorf("save state for %s: %w", comp.Name(), err)
		}

		f.Close()

		manifest.Components[comp.Name()] = manifestEntry{
			Kind:    "modeling.ComponentState",
			Path:    relPath,
			Format:  "json",
			Version: 1,
		}
	}

	return nil
}

func (s *Simulation) saveResources(
	root string,
	manifest *checkpointManifest,
) error {
	resources, err := s.collectResources()
	if err != nil {
		return err
	}

	for _, resource := range resources {
		relPath := checkpointRelPath(
			"resources",
			resource.name,
			resource.fileExtension,
		)
		filePath := checkpointAbsPath(root, relPath)
		f, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("create resource file %s: %w", resource.name, err)
		}

		if err := resource.resource.Save(f); err != nil {
			f.Close()
			return fmt.Errorf("save resource %s: %w", resource.name, err)
		}

		f.Close()

		manifest.Resources[resource.name] = manifestEntry{
			Kind:    resource.kind,
			Path:    relPath,
			Format:  resource.format,
			Version: 1,
		}
	}

	return nil
}

// Load restores simulation state from the given directory path.
//
// The simulation must already be built (topology and connections reconstructed
// from build code) before calling Load.
func (s *Simulation) Load(path string) error {
	manifest, err := readCheckpointManifest(path)
	if errors.Is(err, errCheckpointManifestMissing) {
		manifest = nil
	} else if err != nil {
		return err
	}

	if manifest != nil {
		if err := s.validateManifestForLoad(manifest); err != nil {
			return err
		}
	}

	if err := s.loadMetadata(path); err != nil {
		return err
	}

	if err := s.loadComponentStates(path, manifest); err != nil {
		return err
	}

	if err := s.loadResources(path, manifest); err != nil {
		return err
	}

	s.resetWakeups()

	return nil
}

func (s *Simulation) loadMetadata(path string) error {
	metaData, err := os.ReadFile(filepath.Join(path, "metadata.json"))
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	var meta checkpointMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("unmarshal metadata: %w", err)
	}

	se, ok := s.engine.(*timing.SerialEngine)
	if !ok {
		return fmt.Errorf("Load requires SerialEngine")
	}

	se.SetCurrentTime(meta.EngineTime)
	timing.SetIDGeneratorNextID(meta.IDGeneratorNext)

	return nil
}

func (s *Simulation) loadComponentStates(
	root string,
	manifest *checkpointManifest,
) error {
	for _, comp := range s.components {
		loader, ok := comp.(StateLoader)
		if !ok {
			continue
		}

		var filePath string
		if manifest == nil {
			filePath = filepath.Join(root, "components", comp.Name()+".json")
		} else {
			entry := manifest.Components[comp.Name()]
			filePath = checkpointAbsPath(root, entry.Path)
		}

		f, err := os.Open(filePath)
		if err != nil {
			if manifest == nil && os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("open component file %s: %w", comp.Name(), err)
		}

		if err := loader.LoadState(f); err != nil {
			f.Close()
			return fmt.Errorf("load state for %s: %w", comp.Name(), err)
		}

		f.Close()
	}

	return nil
}

func (s *Simulation) loadResources(
	root string,
	manifest *checkpointManifest,
) error {
	resources, err := s.collectResources()
	if err != nil {
		return err
	}

	for _, resource := range resources {
		var filePath string
		if manifest == nil {
			filePath = filepath.Join(
				root,
				"resources",
				resource.name+resource.fileExtension,
			)
		} else {
			entry := manifest.Resources[resource.name]
			filePath = checkpointAbsPath(root, entry.Path)
		}
		f, err := os.Open(filePath)
		if err != nil {
			if manifest == nil && os.IsNotExist(err) {
				legacyPath := filepath.Join(
					root,
					"storage",
					resource.name+resource.fileExtension,
				)
				f, err = os.Open(legacyPath)
				if os.IsNotExist(err) {
					continue
				}
			}

			if err != nil {
				return fmt.Errorf("open resource file %s: %w", resource.name, err)
			}
		}

		if err := resource.resource.Load(f); err != nil {
			f.Close()
			return fmt.Errorf("load resource %s: %w", resource.name, err)
		}

		f.Close()
	}

	return nil
}

func (s *Simulation) resetWakeups() {
	for _, comp := range s.components {
		if resetter, ok := comp.(WakeupResetter); ok {
			resetter.ResetWakeup()
		}
	}
}

// verifyQuiescence checks that all registered ports have empty buffers.
func (s *Simulation) verifyQuiescence() error {
	for _, p := range s.ports {
		if p.NumIncoming() != 0 {
			return fmt.Errorf("port %s has %d incoming messages",
				p.Name(), p.NumIncoming())
		}

		if p.NumOutgoing() != 0 {
			return fmt.Errorf("port %s has %d outgoing messages",
				p.Name(), p.NumOutgoing())
		}
	}

	return nil
}
