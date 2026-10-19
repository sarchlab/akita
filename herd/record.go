package herd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// Files of a result directory that the tool reads.
const (
	RunRecordFile         = "run.json"                  // the run's record
	MetaFile              = "meta.json"                 // metadata a run writes
	CampaignMetaFile      = "EXPERIMENT_METADATA.json"  // metadata-file campaign
	CampaignFile          = "campaign.json"             // generic campaign
	SensitivityMarkerFile = "SENSITIVITY_CAMPAIGN.json" // sensitivity campaign
)

// MaxRecordBytes bounds the JSON documents of a result directory that are
// read (run.json, meta.json, campaign metadata).
const MaxRecordBytes = 8 << 20

// maxScalarChars bounds the campaign metadata values kept as properties.
const maxScalarChars = 300

// RunRecord is a run's run.json (schema herd-run/1).
type RunRecord struct {
	Schema     string             `json:"schema"`
	Name       string             `json:"name"`
	Fields     map[string]string  `json:"fields,omitempty"`
	Properties []Property         `json:"properties"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	// Status is the run's outcome when it is a campaign cell
	// ("ok", "failed", "running", ...). Optional.
	Status string `json:"status,omitempty"`
}

// Property returns the value of a property, or "".
func (r *RunRecord) Property(name string) string {
	for _, p := range r.Properties {
		if p.Property == name {
			return p.Value
		}
	}

	return ""
}

// ReadRecord reads <dir>/run.json. The error wraps fs.ErrNotExist when the
// run has no record.
func ReadRecord(dir string) (*RunRecord, error) {
	data, err := hostsys.ReadLimited(filepath.Join(dir, RunRecordFile), MaxRecordBytes)
	if err != nil {
		return nil, err
	}

	rec := &RunRecord{}
	if err := json.Unmarshal(data, rec); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, RunRecordFile), err)
	}

	rec.Schema = NormalizeSchema(rec.Schema)
	rec.Properties = validProperties(rec.Properties)

	return rec, nil
}

func validProperties(props []Property) []Property {
	out := props[:0]

	for _, p := range props {
		if p.Property != "" {
			out = append(out, p)
		}
	}

	return out
}

// ParseProperties turns "PROP=VALUE" arguments into properties.
func ParseProperties(args []string) ([]Property, error) {
	props := make([]Property, 0, len(args))

	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("want PROP=VALUE, got %q", a)
		}

		props = append(props, Property{Property: strings.TrimSpace(k), Value: v})
	}

	return props, nil
}

// WriteRecord creates or updates <dir>/run.json. The name is the
// directory's base name and the fields are parsed from it with naming
// (existing fields are kept when the name does not parse). props replace
// properties of the same name; properties the record still lacks are
// filled from meta.json and EXPERIMENT_METADATA.json. Keys of an existing
// run.json that RunRecord does not know are preserved.
func WriteRecord(dir string, naming *Naming, props []Property) (*RunRecord, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(abs, RunRecordFile)
	raw := map[string]json.RawMessage{}

	rec := &RunRecord{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		if err := json.Unmarshal(data, rec); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	rec.Schema = SchemaRun
	rec.Name = filepath.Base(abs)

	if fields := ParseName(rec.Name, naming); fields != nil {
		rec.Fields = fields
	}

	rec.Properties = MergeProperties(validProperties(rec.Properties), props, true)
	rec.Properties = MergeProperties(rec.Properties, MetaProperties(abs), false)

	if err := writeRecordFile(path, raw, rec); err != nil {
		return nil, err
	}

	return rec, nil
}

func writeRecordFile(path string, raw map[string]json.RawMessage, rec *RunRecord) error {
	known, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(known, &fields); err != nil {
		return err
	}

	for k, v := range fields {
		raw[k] = v
	}

	if rec.Fields == nil {
		delete(raw, "fields")
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(path, append(data, '\n'), 0o644)
}

// MergeProperties adds props to base. With replace, a prop replaces the
// base entry of the same name; otherwise only missing names are added.
func MergeProperties(base, props []Property, replace bool) []Property {
	index := make(map[string]int, len(base))
	for i, p := range base {
		index[p.Property] = i
	}

	for _, p := range props {
		if p.Property == "" {
			continue
		}

		i, ok := index[p.Property]

		switch {
		case !ok:
			index[p.Property] = len(base)
			base = append(base, p)
		case replace:
			base[i] = p
		}
	}

	return base
}

// metaProps maps keys of meta.json / EXPERIMENT_METADATA.json to property
// names, in the order the original monitor used.
var metaProps = []struct{ key, prop string }{
	{"commit", "Git Revision"},
	{"git_revision", "Git Revision"},
	{"host", "Host"},
	{"start", "Start Time"},
	{"created_at", "Start Time"},
	{"end", "End Time"},
	{"task", "Task"},
	{"config", "Config Files"},
	{"preset", "Preset"},
	{"gpm_preset", "Preset"},
	{"experiment_count", "Runs"},
	{"exit_code", "Exit Code"},
}

// MetaProperties returns the properties found in a run directory's
// meta.json and EXPERIMENT_METADATA.json.
func MetaProperties(dir string) []Property {
	return MetaPropertiesOf(hostsys.ReadJSONMap(filepath.Join(dir, MetaFile), MaxRecordBytes),
		ScalarMap(hostsys.ReadJSONMap(filepath.Join(dir, CampaignMetaFile), MaxRecordBytes)))
}

// MetaPropertiesOf returns the properties found in metadata documents,
// the first document winning.
func MetaPropertiesOf(sources ...map[string]any) []Property {
	var props []Property

	have := map[string]bool{}

	for _, src := range sources {
		for _, mp := range metaProps {
			v, ok := src[mp.key]
			if !ok || have[mp.prop] {
				continue
			}

			s := PyString(v)
			if s == "" {
				continue
			}

			props = append(props, Property{Property: mp.prop, Value: s})
			have[mp.prop] = true
		}
	}

	return props
}

// ScalarMap keeps the string and number entries of m shorter than 300
// characters (what the original monitor kept of campaign metadata).
func ScalarMap(m map[string]any) map[string]any {
	out := map[string]any{}

	for k, v := range m {
		switch v.(type) {
		case string, float64, bool, json.Number:
			if s := PyString(v); len(s) < maxScalarChars {
				out[k] = v
			}
		}
	}

	return out
}

// PyString formats a JSON value the way Python's str() does for scalars
// (integers without a decimal point); other values are JSON-encoded.
func PyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "True"
		}

		return "False"
	case float64:
		return formatNumber(x)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}

		return string(b)
	}
}

// formatNumber prints a float like Python's str(): integers without a
// fraction, others in the shortest form.
func formatNumber(x float64) string {
	if x == math.Trunc(x) && math.Abs(x) < 1e16 {
		return strconv.FormatInt(int64(x), 10)
	}

	if a := math.Abs(x); a >= 1e-4 && a < 1e16 {
		return strconv.FormatFloat(x, 'f', -1, 64)
	}

	return strconv.FormatFloat(x, 'g', -1, 64)
}
