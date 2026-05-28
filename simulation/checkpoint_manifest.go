package simulation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

const checkpointManifestVersion = 1

var errCheckpointManifestMissing = errors.New("checkpoint manifest missing")

type checkpointManifest struct {
	Version     int                      `json:"version"`
	CreatedBy   string                   `json:"created_by"`
	Engine      manifestEntry            `json:"engine"`
	Globals     manifestEntry            `json:"globals"`
	Components  map[string]manifestEntry `json:"components"`
	Ports       map[string]manifestEntry `json:"ports"`
	Connections map[string]manifestEntry `json:"connections"`
	Resources   map[string]manifestEntry `json:"resources"`
}

type manifestEntry struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Format     string `json:"format"`
	Version    int    `json:"version"`
	SpecHash   string `json:"spec_hash,omitempty"`
	ContentSHA string `json:"content_sha,omitempty"`
}

type checkpointResource struct {
	name          string
	kind          string
	format        string
	fileExtension string
	identity      string
	resource      Resource
}

func newCheckpointManifest() checkpointManifest {
	return checkpointManifest{
		Version:   checkpointManifestVersion,
		CreatedBy: "akita/v5",
		Engine: manifestEntry{
			Kind:    "timing.EngineMetadata",
			Path:    "metadata.json",
			Format:  "json",
			Version: 1,
		},
		Globals: manifestEntry{
			Kind:    "timing.GlobalMetadata",
			Path:    "metadata.json",
			Format:  "json",
			Version: 1,
		},
		Components:  make(map[string]manifestEntry),
		Ports:       make(map[string]manifestEntry),
		Connections: make(map[string]manifestEntry),
		Resources:   make(map[string]manifestEntry),
	}
}

func checkpointRelPath(dir, name, ext string) string {
	return filepath.ToSlash(filepath.Join(dir, url.PathEscape(name)+ext))
}

func checkpointAbsPath(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

func writeCheckpointManifest(root string, manifest checkpointManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(root, "manifest.json"), data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

func readCheckpointManifest(root string) (*checkpointManifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errCheckpointManifestMissing
		}

		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest checkpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	if err := validateManifestBasics(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func validateManifestBasics(manifest *checkpointManifest) error {
	if manifest.Version != checkpointManifestVersion {
		return fmt.Errorf(
			"unsupported checkpoint manifest version %d",
			manifest.Version,
		)
	}

	if manifest.Engine.Path == "" {
		return fmt.Errorf("manifest missing engine path")
	}

	if manifest.Globals.Path == "" {
		return fmt.Errorf("manifest missing globals path")
	}

	if manifest.Components == nil {
		return fmt.Errorf("manifest missing components map")
	}

	if manifest.Ports == nil {
		return fmt.Errorf("manifest missing ports map")
	}

	if manifest.Connections == nil {
		return fmt.Errorf("manifest missing connections map")
	}

	if manifest.Resources == nil {
		return fmt.Errorf("manifest missing resources map")
	}

	return nil
}

func (s *Simulation) validateManifestForLoad(manifest *checkpointManifest) error {
	if err := s.validateComponentManifest(manifest); err != nil {
		return err
	}

	if err := s.validateResourceManifest(manifest); err != nil {
		return err
	}

	return nil
}

func (s *Simulation) validateComponentManifest(manifest *checkpointManifest) error {
	for _, comp := range s.components {
		if _, ok := comp.(StateLoader); !ok {
			continue
		}

		if _, found := manifest.Components[comp.Name()]; !found {
			return fmt.Errorf("manifest missing component %s", comp.Name())
		}
	}

	for name := range manifest.Components {
		idx, found := s.compNameIndex[name]
		if !found {
			return fmt.Errorf("manifest component %s not rebuilt", name)
		}

		if _, ok := s.components[idx].(StateLoader); !ok {
			return fmt.Errorf("manifest component %s is not loadable", name)
		}
	}

	return nil
}

func (s *Simulation) validateResourceManifest(manifest *checkpointManifest) error {
	resources, err := s.collectResources()
	if err != nil {
		return err
	}

	for _, resource := range resources {
		if _, found := manifest.Resources[resource.name]; !found {
			return fmt.Errorf("manifest missing resource %s", resource.name)
		}
	}

	resourceNames := make(map[string]checkpointResource, len(resources))
	for _, resource := range resources {
		resourceNames[resource.name] = resource
	}

	for name, entry := range manifest.Resources {
		resource, found := resourceNames[name]
		if !found {
			return fmt.Errorf("manifest resource %s not rebuilt", name)
		}

		if entry.Kind != resource.kind {
			return fmt.Errorf("manifest resource %s has unsupported kind %s",
				name, entry.Kind)
		}

		if entry.Format != resource.format {
			return fmt.Errorf("manifest resource %s has unsupported format %s",
				name, entry.Format)
		}
	}

	return nil
}

func (s *Simulation) collectResources() ([]checkpointResource, error) {
	resources := make([]checkpointResource, 0, len(s.resources))
	seen := make(map[string]string)

	for _, resource := range s.resources {
		if resource == nil {
			continue
		}

		name := resource.Name()
		if name == "" {
			return nil, fmt.Errorf("resource has empty name")
		}

		kind := resource.Kind()
		if kind == "" {
			return nil, fmt.Errorf("resource %s has empty kind",
				name)
		}

		format := resource.Format()
		if format == "" {
			return nil, fmt.Errorf("resource %s has empty format",
				name)
		}

		fileExtension := resource.FileExtension()
		if fileExtension == "" {
			return nil, fmt.Errorf(
				"resource %s has empty file extension",
				name,
			)
		}

		identity := resource.Identity()
		if identity == "" {
			return nil, fmt.Errorf("resource %s has empty identity",
				name)
		}

		if existingIdentity, found := seen[name]; found {
			if existingIdentity != identity {
				return nil, fmt.Errorf("duplicate resource name %s", name)
			}

			continue
		}

		seen[name] = identity
		resources = append(resources, checkpointResource{
			name:          name,
			kind:          kind,
			format:        format,
			fileExtension: fileExtension,
			identity:      identity,
			resource:      resource,
		})
	}

	return resources, nil
}
