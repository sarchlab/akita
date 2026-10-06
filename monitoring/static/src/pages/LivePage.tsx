import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Flag,
  FlagOff,
  LoaderCircle,
  RefreshCcw,
  Search,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import {
  addWatchedProperty,
  getWatchedProperties,
  removeWatchedProperty,
  subscribeToWatchedProperties,
  type WatchedPropertySampleKind,
  watchedPropertyID,
} from "../utils/watchedProperties";

interface SethNode {
  k: number;
  t: string;
  v?: unknown;
  l?: number;
  o?: number;
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
  page?: number;
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

const PROPERTY_REFRESH_INTERVAL_MS = 2000;
const SLICE_PAGE_SIZE = 50;
const INTEGER_KINDS = new Set([2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]);
const FLOAT_KINDS = new Set([13, 14]);
const MAP_KIND = 21;
const SLICE_KIND = 23;

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

  return node.k === MAP_KIND || node.k === SLICE_KIND || node.k === 25;
}

function monitorSampleKind(node: SethNode | null): WatchedPropertySampleKind | null {
  if (!node) {
    return null;
  }

  if (INTEGER_KINDS.has(node.k) || FLOAT_KINDS.has(node.k)) {
    return "value";
  }

  if (node.k === MAP_KIND || node.k === SLICE_KIND) {
    return "count";
  }

  return null;
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

function nodeLength(node: SethNode | null) {
  if (!node) {
    return null;
  }

  if (typeof node.l === "number" && Number.isFinite(node.l)) {
    return node.l;
  }

  if (Array.isArray(node.v)) {
    return node.v.length;
  }

  return null;
}

function valuePreview(node: SethNode | null) {
  if (node?.k === SLICE_KIND) {
    const length = nodeLength(node);

    if (length !== null) {
      return String(length);
    }
  }

  return primitivePreview(node);
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

function fieldRequestPath(
  componentName: string,
  fieldName: string,
  slicePage?: { offset: number; limit: number },
) {
  const path = `/api/field/${encodeURIComponent(
    JSON.stringify({ comp_name: componentName, field_name: fieldName }),
  )}`;

  if (!slicePage) {
    return path;
  }

  const query = new URLSearchParams({
    slice_offset: String(slicePage.offset),
    slice_limit: String(slicePage.limit),
  });

  return `${path}?${query}`;
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
    const offset = node.k === SLICE_KIND ? (node.o ?? 0) : 0;

    return node.v.map((valueID, index) => ({
      label: String(offset + index),
      path: String(offset + index),
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

function slicePageInfo(node: SethNode | null) {
  if (node?.k !== SLICE_KIND) {
    return null;
  }

  const total = nodeLength(node);
  if (total === null || total <= SLICE_PAGE_SIZE) {
    return null;
  }

  const pageCount = Math.max(1, Math.ceil(total / SLICE_PAGE_SIZE));
  const requestedPage = Math.floor((node.o ?? 0) / SLICE_PAGE_SIZE);
  const page = Math.min(pageCount - 1, Math.max(0, requestedPage));
  const start = page * SLICE_PAGE_SIZE + 1;
  const end = Math.min(total, (page + 1) * SLICE_PAGE_SIZE);

  return { page, pageCount, start, end, total };
}

function useComponentNames() {
  const [components, setComponents] = useState<string[]>([]);

  useEffect(() => {
    fetch("/api/list_components")
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setComponents(Array.isArray(json) ? json.filter((item) => typeof item === "string") : []);
      })
      .catch(() => setComponents([]));
  }, []);

  return { components };
}

async function fetchSnapshot(path: string) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return (await response.json()) as SethSnapshot;
}

function SlicePagination({
  info,
  path,
  node,
  onSlicePageChange,
}: {
  info: NonNullable<ReturnType<typeof slicePageInfo>>;
  path: SethPathSegment[];
  node: SethNode;
  onSlicePageChange: (path: SethPathSegment[], node: SethNode, page: number) => void;
}) {
  const pathID = fieldPath(path);
  const canGoBack = info.page > 0;
  const canGoForward = info.page < info.pageCount - 1;

  return (
    <div className="flex min-h-10 items-center gap-3 border-b bg-slate-50 px-3 py-2 text-xs text-muted-foreground">
      <span className="font-mono tabular-nums">
        {info.start}-{info.end} of {info.total}
      </span>
      <span className="ml-auto font-mono tabular-nums">
        Page {info.page + 1} of {info.pageCount}
      </span>
      <div className="flex items-center gap-1">
        <button
          type="button"
          aria-label={`Previous page ${pathID}`}
          title="Previous page"
          disabled={!canGoBack}
          className="inline-flex h-7 w-7 items-center justify-center rounded text-primary hover:bg-primary/10 disabled:text-muted-foreground disabled:hover:bg-transparent"
          onClick={() => onSlicePageChange(path, node, info.page - 1)}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label={`Next page ${pathID}`}
          title="Next page"
          disabled={!canGoForward}
          className="inline-flex h-7 w-7 items-center justify-center rounded text-primary hover:bg-primary/10 disabled:text-muted-foreground disabled:hover:bg-transparent"
          onClick={() => onSlicePageChange(path, node, info.page + 1)}
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}

function SethRows({
  snapshot,
  node,
  path,
  onSelect,
  onFocus,
  onSlicePageChange,
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
  onFocus: (path: SethPathSegment[], node: SethNode) => void;
  onSlicePageChange: (path: SethPathSegment[], node: SethNode, page: number) => void;
  selectedComponent: string;
  watchedPropertyIDs: Set<string>;
  onToggleWatch: (path: SethPathSegment[], node: SethNode, sampleKind: WatchedPropertySampleKind) => void;
  expandedFields?: Record<string, ExpandedFieldState>;
  depth?: number;
  framed?: boolean;
}) {
  const rows = childRows(snapshot, node);
  const pageInfo = slicePageInfo(node);
  const visibleRows = pageInfo && node.o === undefined ? rows.slice(0, SLICE_PAGE_SIZE) : rows;

  if (!rows.length) {
    return (
      <div className={`${framed ? "rounded border bg-white" : ""} px-3 py-2 text-sm`}>
        <span className="font-mono text-muted-foreground">{valuePreview(node)}</span>
      </div>
    );
  }

  return (
    <div className={framed ? "overflow-hidden rounded border bg-white" : "overflow-hidden"}>
      {pageInfo ? (
        <SlicePagination
          info={pageInfo}
          path={path}
          node={node}
          onSlicePageChange={onSlicePageChange}
        />
      ) : null}
      {visibleRows.map((row) => {
        const child = nodeByID(snapshot, row.valueID);
        const childPath = [...path, row.path];
        const childPathID = fieldPath(childPath);
        const expandedField = expandedFields[childPathID];
        const expandedRoot = rootNode(expandedField?.snapshot ?? null);
        const expandable = isExpandableNode(child);
        const sampleKind = monitorSampleKind(child);
        const watchable = sampleKind !== null;
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
            <div className="grid min-h-11 w-full grid-cols-[minmax(8rem,16rem)_minmax(10rem,1fr)_minmax(7rem,12rem)_2.25rem_2.25rem] items-center gap-3 px-3 py-2 text-sm hover:bg-slate-50">
              <button
                type="button"
                className="contents text-left"
                onClick={() => child && onSelect({ path: childPath, node: child })}
              >
                <span className="min-w-0 truncate font-medium">{row.label}</span>
                <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                  {typeLabel(child)}
                </span>
                <span className="min-w-0 justify-self-end truncate text-right font-mono text-xs tabular-nums text-slate-700">
                  {child?.k === SLICE_KIND || !expandable ? valuePreview(child) : ""}
                </span>
              </button>
              {watchable ? (
                <button
                  type="button"
                  aria-label={`${watched ? "Stop monitoring" : "Monitor"} ${childPathID}`}
                  title={watched ? "Stop monitoring" : "Monitor property"}
                  className={`inline-flex h-8 w-8 items-center justify-center justify-self-center rounded hover:bg-primary/10 ${
                    watched ? "text-primary" : "text-muted-foreground"
                  }`}
                  onClick={() => {
                    if (child && sampleKind) {
                      onSelect({ path: childPath, node: child });
                      onToggleWatch(childPath, child, sampleKind);
                    }
                  }}
                >
                  {watched ? <FlagOff className="h-4 w-4" /> : <Flag className="h-4 w-4" />}
                </button>
              ) : (
                <span className="h-8 w-8 justify-self-center" />
              )}
              {expandable ? (
                <button
                  type="button"
                  aria-label={`${actionLabel} ${childPathID}`}
                  title={actionLabel}
                  className="inline-flex h-8 w-8 items-center justify-center justify-self-center rounded text-primary hover:bg-primary/10 disabled:text-muted-foreground disabled:hover:bg-transparent"
                  disabled={!canToggle}
                  onClick={() => {
                    if (child) {
                      onSelect({ path: childPath, node: child });
                      onFocus(childPath, child);
                    }
                  }}
                >
                  <ActionIcon className={`h-4 w-4 ${expandedField?.loading ? "animate-spin" : ""}`} />
                </button>
              ) : (
                <span className="h-8 w-8 justify-self-center" />
              )}
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
                    onSlicePageChange={onSlicePageChange}
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
                  onSlicePageChange={onSlicePageChange}
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
  onOpenField: (
    sectionID: MonitorSectionID,
    path: SethPathSegment[],
    node: SethNode,
    page?: number,
  ) => void;
  selectedComponent: string;
  watchedPropertyIDs: Set<string>;
  onToggleWatch: (path: SethPathSegment[], node: SethNode, sampleKind: WatchedPropertySampleKind) => void;
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
            onFocus={(path, node) => onOpenField(config.id, path, node)}
            onSlicePageChange={(path, node, page) => onOpenField(config.id, path, node, page)}
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
  const { components } = useComponentNames();
  const [filter, setFilter] = useState("");
  const [selectedComponent, setSelectedComponent] = useState("");
  const [sectionRefreshID, setSectionRefreshID] = useState(0);
  const [autoRefreshProperties, setAutoRefreshProperties] = useState(false);
  const [sections, setSections] = useState<Record<MonitorSectionID, MonitorSectionState>>(() =>
    emptyMonitorSections(),
  );
  const [selected, setSelected] = useState<SelectedNode | null>(null);
  const previousComponentRef = useRef("");
  const [watchedPropertyIDs, setWatchedPropertyIDs] = useState<Set<string>>(() =>
    new Set(getWatchedProperties().map((property) => property.id)),
  );

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
    const componentChanged = previousComponentRef.current !== selectedComponent;
    previousComponentRef.current = selectedComponent;

    if (!selectedComponent) {
      setSelected(null);
      setSections(emptyMonitorSections());
      return;
    }

    let cancelled = false;
    if (componentChanged) {
      setSelected(null);
      setSections(emptyMonitorSections(true));
    }

    MONITOR_SECTIONS.forEach((section) => {
      const loadSection = async () => {
        let lastError: unknown = null;

        for (const fieldName of section.fieldPaths) {
          try {
            const nextSnapshot = await fetchSnapshot(fieldRequestPath(selectedComponent, fieldName));
            if (!cancelled) {
              setSections((previous) => ({
                ...previous,
                [section.id]: {
                  fieldName,
                  snapshot: nextSnapshot,
                  loading: false,
                  error: null,
                  expanded: componentChanged ? {} : previous[section.id].expanded,
                },
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
              snapshot: componentChanged ? null : previous[section.id].snapshot,
              loading: false,
              error: lastError instanceof Error ? lastError.message : `${section.title} unavailable`,
              expanded: componentChanged ? {} : previous[section.id].expanded,
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

  useEffect(() => {
    if (!autoRefreshProperties || !selectedComponent) {
      return;
    }

    const id = window.setInterval(() => {
      setSectionRefreshID((previous) => previous + 1);
    }, PROPERTY_REFRESH_INTERVAL_MS);

    return () => window.clearInterval(id);
  }, [autoRefreshProperties, selectedComponent]);

  useEffect(
    () =>
      subscribeToWatchedProperties(() => {
        setWatchedPropertyIDs(new Set(getWatchedProperties().map((property) => property.id)));
      }),
    [],
  );

  const openSectionField = useCallback(
    (sectionID: MonitorSectionID, path: SethPathSegment[], node: SethNode, page?: number) => {
      if (!selectedComponent) {
        return;
      }

      const fieldName = fieldPath(path);
      const existingField = sections[sectionID]?.expanded[fieldName];
      const pageRequested = page !== undefined;
      const nextPage = page ?? existingField?.page ?? 0;

      if (existingField?.loading) {
        return;
      }

      if (!pageRequested && existingField && !existingField.error) {
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
            [fieldName]: {
              snapshot: existingField?.snapshot ?? null,
              loading: true,
              error: null,
              page: nextPage,
            },
          },
        },
      }));

      const slicePage =
        node.k === SLICE_KIND
          ? { offset: nextPage * SLICE_PAGE_SIZE, limit: SLICE_PAGE_SIZE }
          : undefined;

      fetchSnapshot(fieldRequestPath(selectedComponent, fieldName, slicePage))
        .then((nextSnapshot) => {
          setSections((previous) => ({
            ...previous,
            [sectionID]: {
              ...previous[sectionID],
              expanded: {
                ...previous[sectionID].expanded,
                [fieldName]: {
                  snapshot: nextSnapshot,
                  loading: false,
                  error: null,
                  page: nextPage,
                },
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
                  snapshot: existingField?.snapshot ?? null,
                  loading: false,
                  error: err instanceof Error ? err.message : `Failed to load ${fieldName}`,
                  page: nextPage,
                },
              },
            },
          }));
        });
    },
    [sections, selectedComponent],
  );

  const chooseComponent = (component: string) => {
    setSelectedComponent(component);
    setSelected(null);
  };

  const toggleWatchedProperty = useCallback(
    (path: SethPathSegment[], node: SethNode, sampleKind: WatchedPropertySampleKind) => {
      if (!selectedComponent) {
        return;
      }

      const fieldName = fieldPath(path);
      const id = watchedPropertyID(selectedComponent, fieldName);

      if (watchedPropertyIDs.has(id)) {
        removeWatchedProperty(selectedComponent, fieldName);
      } else {
        addWatchedProperty(
          selectedComponent,
          fieldName,
          sampleKind,
          `${selectedComponent}.${fieldName}`,
        );
      }

      setSelected({ path, node });
    },
    [selectedComponent, watchedPropertyIDs],
  );

  const selectedPath = selected ? fieldPath(selected.path) : "";
  const selectedNode = selected?.node ?? null;

  return (
    <div className="flex h-full flex-col overflow-hidden bg-slate-50">
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
            <label className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
              <input
                type="checkbox"
                className="h-4 w-4 accent-primary"
                checked={autoRefreshProperties}
                disabled={!selectedComponent}
                onChange={(event) => setAutoRefreshProperties(event.target.checked)}
              />
              Auto refresh
            </label>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!selectedComponent}
              onClick={() => setSectionRefreshID((previous) => previous + 1)}
            >
              <RefreshCcw /> Refresh Properties
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
                  <dd className="min-w-0 break-all font-mono text-xs">{valuePreview(selectedNode)}</dd>
                </dl>
              </section>

            </aside>
          </div>
        </section>
      </div>
    </div>
  );
}
