package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

// SimInfoEntry is one key/value row of the exec_info table (e.g. "Command",
// "Start Virtual Time"). The frontend renders these on the main page's
// simulation-info widget.
type SimInfoEntry struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// ListExecInfo returns the exec_info rows in recorded order. A trace without an
// exec_info table yields an empty slice rather than an error.
func (r *SQLiteTraceReader) ListExecInfo(ctx context.Context) []SimInfoEntry {
	entries := []SimInfoEntry{}

	rows, err := r.QueryContext(ctx, "SELECT Property, Value FROM exec_info")
	if err != nil {
		return entries
	}
	defer rows.Close()

	for rows.Next() {
		var e SimInfoEntry
		if err := rows.Scan(&e.Property, &e.Value); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		log.Printf("exec_info query: %v", err)
	}

	return entries
}

func (s *Server) httpSimInfo(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, s.traceReader.ListExecInfo(r.Context()))
}

// TopologyComponent is one component with its spec. Spec is the recorded spec
// embedded as raw JSON (null when the component recorded no spec).
type TopologyComponent struct {
	Name string          `json:"name"`
	Type string          `json:"type"`
	Spec json.RawMessage `json:"spec"`
}

// TopologyPort is one port of a component and the connection it is plugged into.
// Connection is empty for an unconnected port.
type TopologyPort struct {
	Component  string `json:"component"`
	Port       string `json:"port"`
	Connection string `json:"connection"`
}

// Topology is the static structure of a simulation: every component (with its
// spec) and the full port inventory. The connection graph is the subset of
// ports with a non-empty Connection — ports sharing a Connection are that
// connection's endpoints.
type Topology struct {
	Components []TopologyComponent `json:"components"`
	Ports      []TopologyPort      `json:"ports"`
}

// ReadTopology reads the component_spec and port tables. Missing tables (a trace
// recorded before topology recording existed) yield empty slices, so the
// frontend can render an empty state instead of failing.
func (r *SQLiteTraceReader) ReadTopology(ctx context.Context) Topology {
	return Topology{
		Components: r.listComponentSpecs(ctx),
		Ports:      r.listPorts(ctx),
	}
}

func (r *SQLiteTraceReader) listComponentSpecs(
	ctx context.Context,
) []TopologyComponent {
	components := []TopologyComponent{}

	rows, err := r.QueryContext(ctx, "SELECT Name, Type, Spec FROM component_spec")
	if err != nil {
		return components
	}
	defer rows.Close()

	for rows.Next() {
		var name, typ, spec string
		if err := rows.Scan(&name, &typ, &spec); err != nil {
			continue
		}

		c := TopologyComponent{Name: name, Type: typ, Spec: json.RawMessage("null")}
		// Spec is stored as JSON text; embed it as raw JSON so the client gets a
		// real object rather than a quoted string. Fall back to null if a row
		// somehow holds non-JSON.
		if spec != "" && json.Valid([]byte(spec)) {
			c.Spec = json.RawMessage(spec)
		}

		components = append(components, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("component_spec query: %v", err)
	}

	return components
}

func (r *SQLiteTraceReader) listPorts(ctx context.Context) []TopologyPort {
	ports := []TopologyPort{}

	rows, err := r.QueryContext(ctx, "SELECT Component, Port, Connection FROM port")
	if err != nil {
		return ports
	}
	defer rows.Close()

	for rows.Next() {
		var p TopologyPort
		if err := rows.Scan(&p.Component, &p.Port, &p.Connection); err != nil {
			continue
		}
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("port query: %v", err)
	}

	return ports
}

func (s *Server) httpTopology(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, s.traceReader.ReadTopology(r.Context()))
}
