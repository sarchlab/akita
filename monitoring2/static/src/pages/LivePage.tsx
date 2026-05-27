import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Flag, FlagOff, LoaderCircle, Pause, Play, RefreshCcw, Search } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { useEngineTime } from "../hooks/useEngineTime";
import { formatPicosecondsAsNanoseconds } from "../utils/smartValue";
import {
  addWatchedProperty,
  getWatchedProperties,
  removeWatchedProperty,
  subscribeToWatchedProperties,
  watchedPropertyID,
} from "../utils/watchedProperties";

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

type MonitorSectionID = "ports" | "spec" | "state";

interface MonitorSectionConfig {
  id: MonitorSectionID;
  title: string;
  fieldPaths: string[];
}

interface ExpandedFieldState {
  snapshot: SethSnapshot | null;
  loading: boolean;
  error: string | null;
}

interface MonitorSectionState {
  fieldName: string;
  snapshot: SethSnapshot | null;
  loading: boolean;
  error: string | null;
  expanded: Record<string, ExpandedFieldState>;
}

const MONITOR_SECTIONS: MonitorSectionConfig[] = [
  {
    id: "ports",
    title: "Ports",
    fieldPaths: ["TickingComponent.PortOwnerBase.ports", "PortOwnerBase.ports"],
  },
  { id: "spec", title: "Spec", fieldPaths: ["Spec", "Component.Spec"] },
  { id: "state", title: "State", fieldPaths: ["State", "Component.State"] },
];

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

function emptyMonitorSections(loading = false): Record<MonitorSectionID, MonitorSectionState> {
  return MONITOR_SECTIONS.reduce(
    (sections, section) => {
      sections[section.id] = {
        fieldName: section.fieldPaths[0],
        snapshot: null,
        loading,
        error: null,
        expanded: {},
      };
      return sections;
    },
    {} as Record<MonitorSectionID, MonitorSectionState>,
  );
}

function withoutExpandedSubtree(expanded: Record<string, ExpandedFieldState>, fieldName: string) {
  return Object.fromEntries(
    Object.entries(expanded).filter(([key]) => key !== fieldName && !key.startsWith(`${fieldName}.`)),
  );
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
  selectedComponent,
  watchedPropertyIDs,
  onToggleWatch,
  expandedFields = {},
  depth = 0,
  framed = true,
}: {
  snapshot: SethSnapshot;
  node: SethNode;
  path: SethPathSegment[];
  onSelect: (selection: SelectedNode) => void;
  onFocus: (path: SethPathSegment[]) => void;
  selectedComponent: string;
  watchedPropertyIDs: Set<string>;
  onToggleWatch: (path: SethPathSegment[], node: SethNode) => void;
  expandedFields?: Record<string, ExpandedFieldState>;
  depth?: number;
  framed?: boolean;
}) {
  const rows = childRows(snapshot, node);

  if (!rows.length) {
    return (
      <div className={`${framed ? "rounded border bg-white" : ""} px-3 py-2 text-sm`}>
        <span className="font-mono text-muted-foreground">{primitivePreview(node)}</span>
      </div>
    );
  }

  return (
    <div className={framed ? "overflow-hidden rounded border bg-white" : "overflow-hidden"}>
      {rows.map((row) => {
        const child = nodeByID(snapshot, row.valueID);
        const childPath = [...path, row.path];
        const childPathID = fieldPath(childPath);
        const expandedField = expandedFields[childPathID];
        const expandedRoot = rootNode(expandedField?.snapshot ?? null);
        const expandable = isExpandableNode(child);
        const watchable = child !== null && !isContainerNode(child);
        const watched = watchable && watchedPropertyIDs.has(watchedPropertyID(selectedComponent, childPathID));
        const nested = child && isContainerNode(child) && child.v !== undefined && depth < 2;
        const actionLabel = expandedField?.loading
          ? "Loading"
          : expandedField?.error
            ? "Retry"
            : expandedField
              ? "Close"
              : "Open";
        const ActionIcon = expandedField?.loading
          ? LoaderCircle
          : expandedField?.error
            ? RefreshCcw
            : expandedField
              ? ChevronDown
              : ChevronRight;
        const canToggle = expandable && !expandedField?.loading;

        return (
          <div key={`${fieldPath(childPath)}-${row.valueID}`} className="border-b last:border-b-0">
            <div className="grid w-full grid-cols-[minmax(8rem,16rem)_minmax(10rem,1fr)_minmax(8rem,16rem)_auto_auto] items-center gap-3 px-3 py-2 text-sm hover:bg-slate-50">
              <button
                type="button"
                className="col-span-3 grid min-w-0 grid-cols-[minmax(8rem,16rem)_minmax(10rem,1fr)_minmax(8rem,16rem)] items-center gap-3 text-left"
                onClick={() => child && onSelect({ path: childPath, node: child })}
              >
                <span className="min-w-0 truncate font-medium">{row.label}</span>
                <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                  {typeLabel(child)}
                </span>
                <span className="min-w-0 truncate text-right font-mono text-xs text-slate-700">
                  {!expandable ? primitivePreview(child) : ""}
                </span>
              </button>
              {watchable ? (
                <button
                  type="button"
                  aria-label={`${watched ? "Stop monitoring" : "Monitor"} ${childPathID}`}
                  title={watched ? "Stop monitoring" : "Monitor property"}
                  className={`inline-flex h-7 w-7 items-center justify-center justify-self-end rounded hover:bg-primary/10 ${
                    watched ? "text-primary" : "text-muted-foreground"
                  }`}
                  onClick={() => {
                    onSelect({ path: childPath, node: child });
                    onToggleWatch(childPath, child);
                  }}
                >
                  {watched ? <FlagOff className="h-4 w-4" /> : <Flag className="h-4 w-4" />}
                </button>
              ) : (
                <span />
              )}
              {expandable ? (
                <button
                  type="button"
                  aria-label={`${actionLabel} ${childPathID}`}
                  title={actionLabel}
                  className="inline-flex h-7 w-7 items-center justify-center justify-self-end rounded text-primary hover:bg-primary/10 disabled:text-muted-foreground disabled:hover:bg-transparent"
                  disabled={!canToggle}
                  onClick={() => {
                    if (child) {
                      onSelect({ path: childPath, node: child });
                      onFocus(childPath);
                    }
                  }}
                >
                  <ActionIcon className={`h-4 w-4 ${expandedField?.loading ? "animate-spin" : ""}`} />
                </button>
              ) : null}
            </div>
            {expandedField ? (
              <div className="border-t bg-slate-50/60 p-2 pl-6">
                {expandedField.loading ? (
                  <div className="px-3 py-2 text-sm text-muted-foreground">Loading...</div>
                ) : expandedField.error ? (
                  <div className="px-3 py-2 text-sm text-muted-foreground">{expandedField.error}</div>
                ) : expandedRoot && expandedField.snapshot ? (
                  <SethRows
                    snapshot={expandedField.snapshot}
                    node={expandedRoot}
                    path={childPath}
                    onSelect={onSelect}
                    onFocus={onFocus}
                    selectedComponent={selectedComponent}
                    watchedPropertyIDs={watchedPropertyIDs}
                    onToggleWatch={onToggleWatch}
                    expandedFields={expandedFields}
                    depth={depth + 1}
                    framed={framed}
                  />
                ) : null}
              </div>
            ) : null}
            {nested ? (
              <div className="border-t bg-slate-50/60 p-2 pl-6">
                <SethRows
                  snapshot={snapshot}
                  node={child}
                  path={childPath}
                  onSelect={onSelect}
                  onFocus={onFocus}
                  selectedComponent={selectedComponent}
                  watchedPropertyIDs={watchedPropertyIDs}
                  onToggleWatch={onToggleWatch}
                  expandedFields={expandedFields}
                  depth={depth + 1}
                  framed={framed}
                />
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function MonitorSectionView({
  config,
  state,
  onSelect,
  onOpenField,
  selectedComponent,
  watchedPropertyIDs,
  onToggleWatch,
}: {
  config: MonitorSectionConfig;
  state: MonitorSectionState;
  onSelect: (selection: SelectedNode) => void;
  onOpenField: (sectionID: MonitorSectionID, path: SethPathSegment[]) => void;
  selectedComponent: string;
  watchedPropertyIDs: Set<string>;
  onToggleWatch: (path: SethPathSegment[], node: SethNode) => void;
}) {
  const root = rootNode(state.snapshot);

  return (
    <section className="border-b last:border-b-0">
      <div className="flex min-h-10 items-center justify-between gap-3 bg-white px-4 py-2">
        <div className="text-sm font-semibold">{config.title}</div>
        <div className="min-w-0 truncate font-mono text-[11px] text-muted-foreground">{state.fieldName}</div>
      </div>
      <div>
        {state.loading ? (
          <div className="px-4 py-6 text-sm text-muted-foreground">Loading...</div>
        ) : state.error ? (
          <div className="px-4 py-3 text-sm text-muted-foreground">{state.error}</div>
        ) : root && state.snapshot ? (
          <SethRows
            snapshot={state.snapshot}
            node={root}
            path={state.fieldName.split(".")}
            onSelect={onSelect}
            onFocus={(path) => onOpenField(config.id, path)}
            selectedComponent={selectedComponent}
            watchedPropertyIDs={watchedPropertyIDs}
            onToggleWatch={onToggleWatch}
            expandedFields={state.expanded}
            framed={false}
          />
        ) : (
          <div className="px-4 py-6 text-sm text-muted-foreground">
            No {config.title.toLowerCase()} data.
          </div>
        )}
      </div>
    </section>
  );
}

export default function LivePage() {
  const now = useEngineTime(500);
  const { components, refresh: refreshComponents } = useComponentNames();
  const [filter, setFilter] = useState("");
  const [selectedComponent, setSelectedComponent] = useState("");
  const [sectionRefreshID, setSectionRefreshID] = useState(0);
  const [sections, setSections] = useState<Record<MonitorSectionID, MonitorSectionState>>(() =>
    emptyMonitorSections(),
  );
  const [selected, setSelected] = useState<SelectedNode | null>(null);
  const [watchedPropertyIDs, setWatchedPropertyIDs] = useState<Set<string>>(() =>
    new Set(getWatchedProperties().map((property) => property.id)),
  );
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

  useEffect(() => {
    if (!selectedComponent) {
      setSections(emptyMonitorSections());
      return;
    }

    let cancelled = false;
    setSelected(null);
    setSections(emptyMonitorSections(true));

    MONITOR_SECTIONS.forEach((section) => {
      const loadSection = async () => {
        let lastError: unknown = null;

        for (const fieldName of section.fieldPaths) {
          try {
            const nextSnapshot = await fetchSnapshot(fieldRequestPath(selectedComponent, fieldName));
            if (!cancelled) {
              setSections((previous) => ({
                ...previous,
                [section.id]: { fieldName, snapshot: nextSnapshot, loading: false, error: null, expanded: {} },
              }));
            }
            return;
          } catch (err) {
            lastError = err;
          }
        }

        if (!cancelled) {
          setSections((previous) => ({
            ...previous,
            [section.id]: {
              fieldName: section.fieldPaths[0],
              snapshot: null,
              loading: false,
              error: lastError instanceof Error ? lastError.message : `${section.title} unavailable`,
              expanded: {},
            },
          }));
        }
      };

      loadSection();
    });

    return () => {
      cancelled = true;
    };
  }, [sectionRefreshID, selectedComponent]);

  useEffect(
    () =>
      subscribeToWatchedProperties(() => {
        setWatchedPropertyIDs(new Set(getWatchedProperties().map((property) => property.id)));
      }),
    [],
  );

  const openSectionField = useCallback(
    (sectionID: MonitorSectionID, path: SethPathSegment[]) => {
      if (!selectedComponent) {
        return;
      }

      const fieldName = fieldPath(path);
      const existingField = sections[sectionID]?.expanded[fieldName];

      if (existingField?.loading) {
        return;
      }

      if (existingField && !existingField.error) {
        setSections((previous) => ({
          ...previous,
          [sectionID]: {
            ...previous[sectionID],
            expanded: withoutExpandedSubtree(previous[sectionID].expanded, fieldName),
          },
        }));
        return;
      }

      setSections((previous) => ({
        ...previous,
        [sectionID]: {
          ...previous[sectionID],
          expanded: {
            ...previous[sectionID].expanded,
            [fieldName]: { snapshot: null, loading: true, error: null },
          },
        },
      }));

      fetchSnapshot(fieldRequestPath(selectedComponent, fieldName))
        .then((nextSnapshot) => {
          setSections((previous) => ({
            ...previous,
            [sectionID]: {
              ...previous[sectionID],
              expanded: {
                ...previous[sectionID].expanded,
                [fieldName]: { snapshot: nextSnapshot, loading: false, error: null },
              },
            },
          }));
        })
        .catch((err: unknown) => {
          setSections((previous) => ({
            ...previous,
            [sectionID]: {
              ...previous[sectionID],
              expanded: {
                ...previous[sectionID].expanded,
                [fieldName]: {
                  snapshot: null,
                  loading: false,
                  error: err instanceof Error ? err.message : `Failed to load ${fieldName}`,
                },
              },
            },
          }));
        });
    },
    [sections, selectedComponent],
  );

  const runAction = async (label: string, action: () => Promise<void>) => {
    setStatus(`${label}...`);
    try {
      await action();
      setStatus(`${label} complete`);
    } catch (err) {
      setStatus(err instanceof Error ? err.message : `${label} failed`);
    }
  };

  const chooseComponent = (component: string) => {
    setSelectedComponent(component);
    setSelected(null);
  };

  const toggleWatchedProperty = useCallback(
    (path: SethPathSegment[], node: SethNode) => {
      if (!selectedComponent) {
        return;
      }

      const fieldName = fieldPath(path);
      const id = watchedPropertyID(selectedComponent, fieldName);

      if (watchedPropertyIDs.has(id)) {
        removeWatchedProperty(selectedComponent, fieldName);
      } else {
        addWatchedProperty(selectedComponent, fieldName, `${selectedComponent}.${fieldName}`);
      }

      setSelected({ path, node });
    },
    [selectedComponent, watchedPropertyIDs],
  );

  const selectedPath = selected ? fieldPath(selected.path) : "";
  const selectedNode = selected?.node ?? null;

  return (
    <div className="flex h-full flex-col overflow-hidden bg-slate-50">
      <div className="flex min-h-14 flex-wrap items-center gap-2 border-b bg-white px-4 py-2">
        <div className="mr-4 text-sm">
          <span className="text-muted-foreground">Engine time</span>
          <span className="ml-2 font-semibold">{now == null ? "-" : formatPicosecondsAsNanoseconds(now)}</span>
        </div>
        <Button type="button" size="sm" onClick={() => runAction("Continue", () => post("/api/continue"))}>
          <Play /> Continue
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => runAction("Pause", () => post("/api/pause"))}>
          <Pause /> Pause
        </Button>
        <div className="ml-auto text-xs text-muted-foreground">
          {status ? <span>{status}</span> : null}
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
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!selectedComponent}
              onClick={() => setSectionRefreshID((previous) => previous + 1)}
            >
              <RefreshCcw /> Refresh
            </Button>
          </div>

          <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_24rem] overflow-hidden">
            <div className="min-h-0 overflow-auto bg-white">
              {selectedComponent ? (
                <div>
                  {MONITOR_SECTIONS.map((section) => (
                    <MonitorSectionView
                      key={section.id}
                      config={section}
                      state={sections[section.id]}
                      onSelect={setSelected}
                      onOpenField={openSectionField}
                      selectedComponent={selectedComponent}
                      watchedPropertyIDs={watchedPropertyIDs}
                      onToggleWatch={toggleWatchedProperty}
                    />
                  ))}
                </div>
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
                  <dd className="min-w-0 break-all font-mono text-xs">{selectedPath || "-"}</dd>
                  <dt className="text-muted-foreground">Type</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{typeLabel(selectedNode)}</dd>
                  <dt className="text-muted-foreground">Value</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{primitivePreview(selectedNode)}</dd>
                </dl>
              </section>

            </aside>
          </div>
        </section>
      </div>
    </div>
  );
}
