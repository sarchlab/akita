import { useCallback, useEffect, useMemo, useState } from "react";
import { Database, Gauge, Pause, Play, RefreshCcw, Search } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { useEngineTime } from "../hooks/useEngineTime";
import { smartString } from "../utils/smartValue";

interface SethNode {
  k: number;
  t: string;
  v?: unknown;
  l?: number;
}

interface SethSnapshot {
  r: string;
  dict: Record<string, SethNode>;
}

type SethPathSegment = string;

interface SelectedNode {
  path: SethPathSegment[];
  node: SethNode;
}

interface BufferState {
  buffer: string;
  level: number;
  cap: number;
}

function rootNode(snapshot: SethSnapshot | null): SethNode | null {
  if (!snapshot) {
    return null;
  }

  return snapshot.dict[snapshot.r] ?? null;
}

function nodeByID(snapshot: SethSnapshot, id: string | number): SethNode | null {
  return snapshot.dict[String(id)] ?? null;
}

function isContainerNode(node: SethNode | null) {
  if (!node) {
    return false;
  }

  return node.k === 21 || node.k === 23 || node.k === 25;
}

function isExpandableNode(node: SethNode | null) {
  if (!node) {
    return false;
  }

  return isContainerNode(node) && node.v === undefined;
}

function primitivePreview(node: SethNode | null) {
  if (!node) {
    return "null";
  }

  if (node.v === undefined) {
    return node.l === undefined ? node.t : `${node.t}, len ${node.l}`;
  }

  if (node.v === null) {
    return "null";
  }

  if (typeof node.v === "string") {
    return node.v;
  }

  if (typeof node.v === "number" || typeof node.v === "boolean") {
    return String(node.v);
  }

  if (Array.isArray(node.v)) {
    return `${node.t}, len ${node.l ?? node.v.length}`;
  }

  if (typeof node.v === "object") {
    return node.l === undefined ? node.t : `${node.t}, len ${node.l}`;
  }

  return String(node.v);
}

function typeLabel(node: SethNode | null) {
  if (!node) {
    return "null";
  }

  if (node.l === undefined) {
    return node.t;
  }

  return `${node.t} (${node.l})`;
}

function fieldPath(path: SethPathSegment[]) {
  return path.join(".");
}

function fieldRequestPath(componentName: string, fieldName: string) {
  return `/api/field/${encodeURIComponent(
    JSON.stringify({ comp_name: componentName, field_name: fieldName }),
  )}`;
}

function childRows(snapshot: SethSnapshot, node: SethNode) {
  if (!node.v) {
    return [];
  }

  if (Array.isArray(node.v)) {
    return node.v.map((valueID, index) => ({
      label: String(index),
      path: String(index),
      valueID: String(valueID),
    }));
  }

  if (typeof node.v === "object") {
    if (node.k === 21) {
      return Object.entries(node.v as Record<string, string>).map(([keyID, valueID]) => {
        const keyNode = nodeByID(snapshot, keyID);
        return {
          label: primitivePreview(keyNode),
          path: primitivePreview(keyNode),
          valueID: String(valueID),
        };
      });
    }

    return Object.entries(node.v as Record<string, string>).map(([label, valueID]) => ({
      label,
      path: label,
      valueID: String(valueID),
    }));
  }

  return [];
}

function bufferPercent(buffer: BufferState) {
  if (!buffer.cap) {
    return 0;
  }

  return Math.min(1, Math.max(0, buffer.level / buffer.cap));
}

function useComponentNames() {
  const [components, setComponents] = useState<string[]>([]);

  const refresh = useCallback(() => {
    fetch("/api/list_components")
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setComponents(Array.isArray(json) ? json.filter((item) => typeof item === "string") : []);
      })
      .catch(() => setComponents([]));
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 2000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { components, refresh };
}

function useBuffers(sortMethod: "percent" | "level") {
  const [buffers, setBuffers] = useState<BufferState[]>([]);

  const refresh = useCallback(() => {
    fetch(`/api/hangdetector/buffers?sort=${sortMethod}&limit=12`)
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setBuffers(Array.isArray(json) ? json.filter(isBufferState) : []);
      })
      .catch(() => setBuffers([]));
  }, [sortMethod]);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1500);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { buffers, refresh };
}

function isBufferState(value: unknown): value is BufferState {
  if (!value || typeof value !== "object") {
    return false;
  }

  const buffer = value as Partial<BufferState>;
  return (
    typeof buffer.buffer === "string" &&
    typeof buffer.level === "number" &&
    typeof buffer.cap === "number"
  );
}

function useTraceStatus() {
  const [isTracing, setIsTracing] = useState(false);

  const refresh = useCallback(() => {
    fetch("/api/trace/is_tracing")
      .then((response) => (response.ok ? response.json() : null))
      .then((json: unknown) => {
        if (json && typeof json === "object" && "isTracing" in json) {
          setIsTracing(Boolean((json as { isTracing: unknown }).isTracing));
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { isTracing, refresh };
}

async function post(path: string) {
  const response = await fetch(path, { method: "POST" });
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }
}

async function fetchSnapshot(path: string) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return (await response.json()) as SethSnapshot;
}

function SethRows({
  snapshot,
  node,
  path,
  onSelect,
  onFocus,
  depth = 0,
}: {
  snapshot: SethSnapshot;
  node: SethNode;
  path: SethPathSegment[];
  onSelect: (selection: SelectedNode) => void;
  onFocus: (path: SethPathSegment[]) => void;
  depth?: number;
}) {
  const rows = childRows(snapshot, node);

  if (!rows.length) {
    return (
      <div className="rounded border bg-white px-3 py-2 text-sm">
        <span className="font-mono text-muted-foreground">{primitivePreview(node)}</span>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded border bg-white">
      {rows.map((row) => {
        const child = nodeByID(snapshot, row.valueID);
        const childPath = [...path, row.path];
        const expandable = isExpandableNode(child);
        const nested = child && isContainerNode(child) && child.v !== undefined && depth < 2;

        return (
          <div key={`${fieldPath(childPath)}-${row.valueID}`} className="border-b last:border-b-0">
            <button
              type="button"
              className="grid w-full grid-cols-[minmax(8rem,16rem)_minmax(10rem,1fr)_auto] items-center gap-3 px-3 py-2 text-left text-sm hover:bg-slate-50"
              onClick={() => child && onSelect({ path: childPath, node: child })}
              onDoubleClick={() => expandable && onFocus(childPath)}
            >
              <span className="min-w-0 truncate font-medium">{row.label}</span>
              <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                {typeLabel(child)}
              </span>
              {expandable ? (
                <span className="text-xs font-medium text-primary">Open</span>
              ) : (
                <span className="max-w-64 truncate text-right font-mono text-xs text-slate-700">
                  {primitivePreview(child)}
                </span>
              )}
            </button>
            {nested ? (
              <div className="border-t bg-slate-50/60 p-2 pl-6">
                <SethRows
                  snapshot={snapshot}
                  node={child}
                  path={childPath}
                  onSelect={onSelect}
                  onFocus={onFocus}
                  depth={depth + 1}
                />
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

export default function LivePage() {
  const now = useEngineTime(500);
  const { components, refresh: refreshComponents } = useComponentNames();
  const { isTracing, refresh: refreshTraceStatus } = useTraceStatus();
  const [bufferSort, setBufferSort] = useState<"percent" | "level">("percent");
  const { buffers, refresh: refreshBuffers } = useBuffers(bufferSort);
  const [filter, setFilter] = useState("");
  const [selectedComponent, setSelectedComponent] = useState("");
  const [focusPath, setFocusPath] = useState<SethPathSegment[]>([]);
  const [snapshot, setSnapshot] = useState<SethSnapshot | null>(null);
  const [selected, setSelected] = useState<SelectedNode | null>(null);
  const [loadingSnapshot, setLoadingSnapshot] = useState(false);
  const [snapshotError, setSnapshotError] = useState<string | null>(null);
  const [status, setStatus] = useState("");

  useEffect(() => {
    if (!selectedComponent && components.length) {
      setSelectedComponent(components[0]);
    }
  }, [components, selectedComponent]);

  const visibleComponents = useMemo(() => {
    if (!filter) {
      return components;
    }

    return components.filter((component) => component.includes(filter));
  }, [components, filter]);

  const loadComponent = useCallback((component: string, path: SethPathSegment[] = []) => {
    if (!component) {
      setSnapshot(null);
      return;
    }

    setLoadingSnapshot(true);
    setSnapshotError(null);
    setSelected(null);

    const requestPath = path.length
      ? fieldRequestPath(component, fieldPath(path))
      : `/api/component/${encodeURIComponent(component)}`;

    fetchSnapshot(requestPath)
      .then((nextSnapshot) => {
        setSnapshot(nextSnapshot);
        setFocusPath(path);
      })
      .catch((err: unknown) => {
        setSnapshot(null);
        setSnapshotError(err instanceof Error ? err.message : "Failed to load component");
      })
      .finally(() => setLoadingSnapshot(false));
  }, []);

  useEffect(() => {
    loadComponent(selectedComponent, focusPath);
  }, [focusPath, loadComponent, selectedComponent]);

  const runAction = async (label: string, action: () => Promise<void>) => {
    setStatus(`${label}...`);
    try {
      await action();
      setStatus(`${label} complete`);
    } catch (err) {
      setStatus(err instanceof Error ? err.message : `${label} failed`);
    }
  };

  const refreshAnalysis = () => {
    refreshBuffers();
  };

  const chooseComponent = (component: string) => {
    setSelectedComponent(component);
    setFocusPath([]);
    setSelected(null);
  };

  const root = rootNode(snapshot);
  const selectedPath = selected ? fieldPath(selected.path) : fieldPath(focusPath);
  const selectedNode = selected?.node ?? root;

  return (
    <div className="flex h-full flex-col overflow-hidden bg-slate-50">
      <div className="flex min-h-14 flex-wrap items-center gap-2 border-b bg-white px-4 py-2">
        <div className="mr-4 text-sm">
          <span className="text-muted-foreground">Engine time</span>
          <span className="ml-2 font-semibold">{now == null ? "-" : smartString(now)}</span>
        </div>
        <Button type="button" size="sm" onClick={() => runAction("Continue", () => post("/api/continue"))}>
          <Play /> Continue
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => runAction("Pause", () => post("/api/pause"))}>
          <Pause /> Pause
        </Button>
        <div className="mx-2 h-6 w-px bg-border" />
        <Button
          type="button"
          size="sm"
          variant={isTracing ? "outline" : "default"}
          onClick={() => runAction("Start tracing", () => post("/api/trace/start").then(refreshTraceStatus))}
        >
          <Database /> Start Tracing
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => runAction("Pause tracing", () => post("/api/trace/end").then(refreshTraceStatus))}
        >
          <Pause /> Pause Tracing
        </Button>
        <div className="ml-auto text-xs text-muted-foreground">
          Tracing: <span className="font-semibold text-foreground">{isTracing ? "on" : "off"}</span>
          {status ? <span className="ml-3">{status}</span> : null}
        </div>
      </div>

      <div className="flex min-h-0 flex-1 overflow-hidden">
        <aside className="flex w-80 shrink-0 flex-col border-r bg-white">
          <div className="border-b p-3">
            <div className="mb-2 flex items-center gap-2">
              <Search className="h-4 w-4 text-muted-foreground" />
              <Input
                value={filter}
                placeholder="Filter components"
                onChange={(event) => setFilter(event.target.value)}
              />
              <Button type="button" variant="outline" size="icon" onClick={refreshComponents}>
                <RefreshCcw />
              </Button>
            </div>
            <div className="text-xs text-muted-foreground">{components.length} components</div>
          </div>
          <div className="min-h-0 flex-1 overflow-auto">
            {visibleComponents.length ? (
              visibleComponents.map((component) => (
                <button
                  key={component}
                  type="button"
                  className={`block w-full border-b px-3 py-2 text-left text-sm hover:bg-slate-50 ${
                    component === selectedComponent ? "bg-primary/10 font-semibold text-primary" : "bg-white"
                  }`}
                  onClick={() => chooseComponent(component)}
                >
                  <span className="block truncate">{component}</span>
                </button>
              ))
            ) : (
              <div className="p-6 text-center text-sm text-muted-foreground">No components available.</div>
            )}
          </div>
        </aside>

        <section className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="flex min-h-12 items-center gap-3 border-b bg-white px-4 py-2">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-semibold">{selectedComponent || "No component selected"}</div>
              <div className="truncate text-xs text-muted-foreground">{selectedPath || "root"}</div>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!selectedComponent}
              onClick={() => loadComponent(selectedComponent, focusPath)}
            >
              <RefreshCcw /> Refresh
            </Button>
          </div>

          <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_24rem] overflow-hidden">
            <div className="min-h-0 overflow-auto bg-slate-50 p-3">
              {loadingSnapshot ? (
                <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Loading...</div>
              ) : snapshotError ? (
                <div className="rounded border border-destructive/30 bg-white p-4 text-sm text-destructive">
                  {snapshotError}
                </div>
              ) : root && snapshot ? (
                <SethRows
                  snapshot={snapshot}
                  node={root}
                  path={focusPath}
                  onSelect={setSelected}
                  onFocus={(path) => setFocusPath(path)}
                />
              ) : (
                <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                  Select a component.
                </div>
              )}
            </div>

            <aside className="min-h-0 overflow-auto border-l bg-white">
              <section className="border-b p-4">
                <div className="text-sm font-semibold">Selection</div>
                <dl className="mt-3 grid grid-cols-[5rem_1fr] gap-y-2 text-sm">
                  <dt className="text-muted-foreground">Path</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{selectedPath || "root"}</dd>
                  <dt className="text-muted-foreground">Type</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{typeLabel(selectedNode)}</dd>
                  <dt className="text-muted-foreground">Value</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{primitivePreview(selectedNode)}</dd>
                </dl>
                {selected && isExpandableNode(selected.node) ? (
                  <Button type="button" className="mt-4 w-full" onClick={() => setFocusPath(selected.path)}>
                    Open Field
                  </Button>
                ) : null}
              </section>

              <section className="border-b p-4">
                <div className="mb-3 flex items-center gap-2">
                  <Gauge className="h-4 w-4 text-muted-foreground" />
                  <div className="text-sm font-semibold">Buffer Level Analysis</div>
                  <Button type="button" size="icon" variant="outline" className="ml-auto" onClick={refreshAnalysis}>
                    <RefreshCcw />
                  </Button>
                </div>
                <div className="mb-3 flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant={bufferSort === "percent" ? "default" : "outline"}
                    onClick={() => setBufferSort("percent")}
                  >
                    Percent
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={bufferSort === "level" ? "default" : "outline"}
                    onClick={() => setBufferSort("level")}
                  >
                    Level
                  </Button>
                </div>
                <div className="space-y-2">
                  {buffers.length ? (
                    buffers.map((buffer) => {
                      const percent = bufferPercent(buffer);
                      return (
                        <div key={buffer.buffer} className="rounded border bg-white p-2">
                          <div className="flex items-center justify-between gap-2 text-xs">
                            <span className="min-w-0 truncate font-medium">{buffer.buffer}</span>
                            <span className="shrink-0 text-muted-foreground">
                              {buffer.level}/{buffer.cap}
                            </span>
                          </div>
                          <div className="mt-2 h-1.5 rounded-full bg-slate-200">
                            <div className="h-1.5 rounded-full bg-amber-500" style={{ width: `${percent * 100}%` }} />
                          </div>
                        </div>
                      );
                    })
                  ) : (
                    <div className="rounded border bg-slate-50 p-3 text-center text-xs text-muted-foreground">
                      No buffers reported.
                    </div>
                  )}
                </div>
              </section>

            </aside>
          </div>
        </section>
      </div>
    </div>
  );
}
