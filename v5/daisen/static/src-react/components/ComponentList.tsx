import { useState, useMemo } from "react";
import { Link } from "react-router";

interface ComponentListProps {
  /** Component names from /api/compnames. */
  names: string[];
  /** Whether names are still loading. */
  loading: boolean;
  /** Optional error message. */
  error: string | null;
}

/**
 * Searchable, filterable list of simulation components.
 * Each item links to /component?name=X.
 */
export default function ComponentList({
  names,
  loading,
  error,
}: ComponentListProps) {
  const [filter, setFilter] = useState("");

  const filtered = useMemo(() => {
    if (!filter) return names;
    try {
      const re = new RegExp(filter, "i");
      return names.filter((n) => re.test(n));
    } catch {
      // If invalid regex, fall back to plain includes
      const lower = filter.toLowerCase();
      return names.filter((n) => n.toLowerCase().includes(lower));
    }
  }, [names, filter]);

  if (loading) {
    return (
      <div className="text-center py-4">
        <div className="spinner-border spinner-border-sm text-primary" />
        <span className="ms-2 text-muted">Loading components…</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="alert alert-danger" role="alert">
        Failed to load components: {error}
      </div>
    );
  }

  return (
    <div>
      {/* Filter input */}
      <div className="mb-2">
        <input
          type="text"
          className="form-control form-control-sm"
          placeholder="Filter components (regex supported)…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {/* Count */}
      <p className="text-muted small mb-2">
        Showing {filtered.length} of {names.length} components
      </p>

      {/* List */}
      {filtered.length === 0 ? (
        <div className="alert alert-warning py-2">No matching components.</div>
      ) : (
        <div
          className="list-group list-group-flush"
          style={{ maxHeight: "calc(100vh - 280px)", overflowY: "auto" }}
        >
          {filtered.map((name) => (
            <Link
              key={name}
              to={`/component?name=${encodeURIComponent(name)}`}
              className="list-group-item list-group-item-action py-2 px-3"
              style={{ fontSize: 13 }}
            >
              {name}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
