import { useCallback, useEffect, useState } from "react";
import {
  VarKind,
  isContainerKind,
  isDirectKind,
  isMonitorableKind,
} from "../../types/gotypes";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/** A single entry in the dict returned by the backend. */
interface DictEntry {
  /** VarKind numeric value */
  k: number;
  /** Go type name */
  t: string;
  /** Value — scalar string, object (struct/map), or array (slice) */
  v: unknown;
  /** Length for container types */
  l?: number;
}

/** Dict is keyed by opaque reference strings. */
type Dict = Record<string, DictEntry>;

/** API response shape for /api/component/{name} and /api/field/{req}. */
interface ApiResponse {
  dict: Dict;
  r: string;
}

interface ComponentDetailViewProps {
  componentName: string;
  onMonitor?: (componentName: string, keyChain: string) => void;
}

/* ------------------------------------------------------------------ */
/*  FieldRow — a single expandable/collapsible field                   */
/* ------------------------------------------------------------------ */

function FieldRow({
  fieldKey,
  dict,
  refKey,
  componentName,
  keyChain,
  fieldPrefix,
  onMonitor,
}: {
  fieldKey: string;
  dict: Dict;
  refKey: string;
  componentName: string;
  keyChain: string;
  fieldPrefix: string;
  onMonitor?: (componentName: string, keyChain: string) => void;
}) {
  const entry = dict[refKey];
  if (!entry) return null;

  const kind = entry.k;
  const typeName = entry.t;
  const value = entry.v;
  const length = entry.l;
  const direct = isDirectKind(kind);
  const container = isContainerKind(kind);
  const monitorable = isMonitorableKind(kind);

  const childKeyChain = `${keyChain}.${fieldKey}`;

  const [expanded, setExpanded] = useState(false);
  const [subDict, setSubDict] = useState<Dict | null>(null);
  const [subRoot, setSubRoot] = useState<DictEntry | null>(null);
  const [subFieldPrefix, setSubFieldPrefix] = useState("");
  const [fetching, setFetching] = useState(false);
  const [flagged, setFlagged] = useState(false);

  const toggleExpand = useCallback(() => {
    if (direct) return;

    if (!expanded && !subDict) {
      // Need to fetch sub-field
      setFetching(true);
      const fieldPath = `${fieldPrefix}${fieldKey}`;
      const req = JSON.stringify({
        comp_name: componentName,
        field_name: fieldPath,
      });

      fetch(`/api/field/${req}`)
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.json();
        })
        .then((data: ApiResponse) => {
          setSubDict(data.dict);
          setSubRoot(data.dict[data.r]);
          setSubFieldPrefix(`${fieldPath}.`);
          setFetching(false);
          setExpanded(true);
        })
        .catch((err) => {
          console.error("Error fetching field:", err);
          setFetching(false);
        });
    } else {
      setExpanded((prev) => !prev);
    }
  }, [direct, expanded, subDict, fieldPrefix, fieldKey, componentName]);

  const toggleFlag = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      const next = !flagged;
      setFlagged(next);
      if (onMonitor) {
        onMonitor(componentName, childKeyChain);
      }
    },
    [flagged, onMonitor, componentName, childKeyChain],
  );

  return (
    <tr>
      <td
        onClick={direct ? undefined : toggleExpand}
        style={{ cursor: direct ? "default" : "pointer" }}
      >
        {/* Title row */}
        <div className="d-flex align-items-center gap-1 flex-wrap">
          {/* Chevron or dash */}
          {direct ? (
            <span className="text-muted" style={{ width: 14, textAlign: "center" }}>
              –
            </span>
          ) : (
            <i
              className={`fa-solid fa-xs ${
                expanded ? "fa-chevron-down" : "fa-chevron-right"
              }`}
              style={{ width: 14, textAlign: "center" }}
            />
          )}

          {/* Flag button for monitorable kinds */}
          {monitorable && (
            <span
              className="flag-button"
              onClick={toggleFlag}
              role="button"
              style={{ cursor: "pointer" }}
            >
              <i className={`fa-flag ${flagged ? "fa-solid text-warning" : "fa-regular text-muted"}`} />
            </span>
          )}

          {/* Field name */}
          <span className="fw-semibold">{fieldKey}</span>

          {/* Type */}
          <span className="text-muted small">{typeName}</span>

          {/* Direct value */}
          {direct && (
            <span className="ms-1 text-primary">{String(value)}</span>
          )}

          {/* Container length */}
          {container && length !== undefined && (
            <span className="badge bg-secondary ms-1">{length}</span>
          )}

          {fetching && (
            <span className="spinner-border spinner-border-sm ms-1" />
          )}
        </div>

        {/* Sub-content */}
        {expanded && subDict && subRoot && (
          <div className="ms-3 mt-1">
            <ContentView
              entry={subRoot}
              dict={subDict}
              componentName={componentName}
              keyChain={childKeyChain}
              fieldPrefix={subFieldPrefix}
              onMonitor={onMonitor}
            />
          </div>
        )}
      </td>
    </tr>
  );
}

/* ------------------------------------------------------------------ */
/*  ContentView — renders struct/map/slice/direct                      */
/* ------------------------------------------------------------------ */

function ContentView({
  entry,
  dict,
  componentName,
  keyChain,
  fieldPrefix,
  onMonitor,
}: {
  entry: DictEntry;
  dict: Dict;
  componentName: string;
  keyChain: string;
  fieldPrefix: string;
  onMonitor?: (componentName: string, keyChain: string) => void;
}) {
  const kind = entry.k;
  const value = entry.v;

  if (isDirectKind(kind)) {
    return <span className="text-primary">{String(value)}</span>;
  }

  // For struct, map, slice — value is an object or array with ref keys
  let fields: [string, string][] = [];

  if (kind === VarKind.Map && Array.isArray(value)) {
    // Map: value is array of ref keys indexed by number
    fields = (value as string[]).map((ref, i) => [String(i), ref]);
  } else if (typeof value === "object" && value !== null) {
    // Struct or Slice: value is an object with named keys → ref values
    const obj = value as Record<string, string>;
    fields = Object.keys(obj)
      .sort()
      .map((k) => [k, obj[k]]);
  }

  if (fields.length === 0) {
    return <span className="text-muted">(empty)</span>;
  }

  return (
    <table className="table table-sm table-borderless mb-0" style={{ fontSize: "0.85rem" }}>
      <tbody>
        {fields.map(([key, ref]) => (
          <FieldRow
            key={key}
            fieldKey={key}
            dict={dict}
            refKey={ref}
            componentName={componentName}
            keyChain={keyChain}
            fieldPrefix={fieldPrefix}
            onMonitor={onMonitor}
          />
        ))}
      </tbody>
    </table>
  );
}

/* ------------------------------------------------------------------ */
/*  Main ComponentDetailView                                           */
/* ------------------------------------------------------------------ */

/**
 * Fetches /api/component/{name}, then recursively displays the struct
 * fields. Sub-fields are lazily fetched via /api/field/{request}.
 *
 * Only meaningful in live mode — the parent should guard rendering.
 */
export default function ComponentDetailView({
  componentName,
  onMonitor,
}: ComponentDetailViewProps) {
  const [dict, setDict] = useState<Dict | null>(null);
  const [rootEntry, setRootEntry] = useState<DictEntry | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    setDict(null);
    setRootEntry(null);

    fetch(`/api/component/${componentName}`, { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((data: ApiResponse) => {
        setDict(data.dict);
        setRootEntry(data.dict[data.r]);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, [componentName]);

  const handleTick = useCallback(() => {
    fetch(`/api/tick/${componentName}`).catch(console.error);
  }, [componentName]);

  if (loading) {
    return (
      <div className="p-3">
        <div className="spinner-border spinner-border-sm me-2" />
        Loading {componentName}…
      </div>
    );
  }

  if (error) {
    return (
      <div className="alert alert-danger m-2">
        Error loading {componentName}: {error}
      </div>
    );
  }

  if (!dict || !rootEntry) {
    return <div className="p-3 text-muted">No data available.</div>;
  }

  return (
    <div className="component-detail">
      {/* Header */}
      <div className="d-flex align-items-center justify-content-between mb-2">
        <h5 className="mb-0">{componentName}</h5>
        <button
          type="button"
          className="btn btn-success btn-sm"
          onClick={handleTick}
        >
          Tick
        </button>
      </div>

      {/* Field tree */}
      <div className="component-detail-content">
        <ContentView
          entry={rootEntry}
          dict={dict}
          componentName={componentName}
          keyChain=""
          fieldPrefix=""
          onMonitor={onMonitor}
        />
      </div>
    </div>
  );
}
