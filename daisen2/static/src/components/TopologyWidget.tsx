import { useMemo, useState } from "react";
import WidgetCard from "./WidgetCard";
import { useTopology } from "../hooks/useTopology";
import type { Topology, TopologyComponent, TopologyPort } from "../types/overview";

const EMPTY_TOPOLOGY: Topology = { components: [], ports: [] };

const COL_GAP = 210;
const ROW_GAP = 82;
const PAD = 36;
const LABEL_W = 150;
const NODE_R = 8;
const NODE_COLOR = "#2c7bb6";
const SELECTED_COLOR = "#f97316";

interface Edge {
  source: string;
  target: string;
  sourcePort: string;
  targetPort: string;
  connection: string;
}

interface PositionedNode {
  id: string;
  x: number;
  y: number;
  component: TopologyComponent | null;
}

interface Layout {
  nodes: PositionedNode[];
  edges: Edge[];
  width: number;
  height: number;
}

// shortPort drops the owning-component prefix: "L1Cache.Top" -> "Top".
function shortPort(full: string): string {
  const dot = full.lastIndexOf(".");
  return dot >= 0 ? full.slice(dot + 1) : full;
}

// A ".Top" port points upstream (toward the requester), so its owner is the
// child in the hierarchy. This mirrors Akita's port-naming convention and lets
// us lay the graph out as a tidy tree instead of a force cloud.
function isTopPort(port: string): boolean {
  return shortPort(port).toLowerCase() === "top";
}

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

// buildLayout derives a deterministic, layered tree layout from the topology.
// Edges come from ports sharing a connection; parent/child orientation comes
// from the ".Top" convention, with a BFS fallback for anything ambiguous.
function buildLayout(topology: Topology): Layout {
  const compByName = new Map(topology.components.map((c) => [c.name, c]));

  const ids: string[] = [];
  const seen = new Set<string>();
  const addId = (id: string) => {
    if (!seen.has(id)) {
      seen.add(id);
      ids.push(id);
    }
  };
  topology.components.forEach((c) => addId(c.name));
  topology.ports.forEach((p) => addId(p.component));

  const members = membersByConnection(topology);
  const edges: Edge[] = [];
  members.forEach((list, connection) => {
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        edges.push({
          source: list[i].component,
          sourcePort: list[i].port,
          target: list[j].component,
          targetPort: list[j].port,
          connection,
        });
      }
    }
  });

  const children = new Map<string, string[]>();
  ids.forEach((id) => children.set(id, []));
  const parent = new Map<string, string>();
  edges.forEach((e) => {
    const sTop = isTopPort(e.sourcePort);
    const tTop = isTopPort(e.targetPort);
    let p: string | null = null;
    let c: string | null = null;
    if (tTop && !sTop) {
      p = e.source;
      c = e.target;
    } else if (sTop && !tTop) {
      p = e.target;
      c = e.source;
    }
    if (p && c && p !== c && !parent.has(c)) {
      parent.set(c, p);
      children.get(p)!.push(c);
    }
  });

  let roots = ids.filter((id) => !parent.has(id));
  if (roots.length === 0 && ids.length > 0) roots = [ids[0]];

  // Tidy-tree DFS: leaves get sequential columns, parents center over children.
  const depth = new Map<string, number>();
  const order = new Map<string, number>();
  const visited = new Set<string>();
  let nextLeaf = 0;
  const place = (id: string, d: number): number => {
    visited.add(id);
    depth.set(id, d);
    const kids = (children.get(id) ?? []).filter((k) => !visited.has(k));
    if (kids.length === 0) {
      const x = nextLeaf++;
      order.set(id, x);
      return x;
    }
    const xs = kids.map((k) => place(k, d + 1));
    const x = (Math.min(...xs) + Math.max(...xs)) / 2;
    order.set(id, x);
    return x;
  };
  roots.forEach((r) => place(r, 0));
  ids.forEach((id) => {
    if (!visited.has(id)) place(id, 0);
  });

  const nodes: PositionedNode[] = ids.map((id) => ({
    id,
    component: compByName.get(id) ?? null,
    x: PAD + (order.get(id) ?? 0) * COL_GAP,
    y: PAD + (depth.get(id) ?? 0) * ROW_GAP,
  }));

  const maxX = nodes.reduce((m, n) => Math.max(m, n.x), 0);
  const maxY = nodes.reduce((m, n) => Math.max(m, n.y), 0);

  return { nodes, edges, width: maxX + LABEL_W + PAD, height: maxY + PAD };
}

function membersByConnection(
  topology: Topology,
): Map<string, { component: string; port: string }[]> {
  const members = new Map<string, { component: string; port: string }[]>();
  topology.ports.forEach((p) => {
    if (!p.connection) return;
    const arr = members.get(p.connection) ?? [];
    arr.push({ component: p.component, port: p.port });
    members.set(p.connection, arr);
  });
  return members;
}

interface TopologyWidgetProps {
  expandHref?: string;
}

export default function TopologyWidget({ expandHref }: TopologyWidgetProps) {
  const { data, loading, error } = useTopology();
  const topology = data ?? EMPTY_TOPOLOGY;
  const [selected, setSelected] = useState<string | null>(null);

  const { nodes, edges, width, height } = useMemo(
    () => buildLayout(topology),
    [topology],
  );
  const nodeById = useMemo(
    () => new Map(nodes.map((n) => [n.id, n])),
    [nodes],
  );
  const members = useMemo(() => membersByConnection(topology), [topology]);

  const selectedComponent =
    nodes.find((n) => n.id === selected)?.component ?? null;
  const selectedPorts = useMemo(
    () => (selected ? topology.ports.filter((p) => p.component === selected) : []),
    [selected, topology],
  );

  const peerOf = (port: TopologyPort) => {
    if (!port.connection) return null;
    const mem = members.get(port.connection) ?? [];
    const others = mem.filter(
      (x) => !(x.component === port.component && x.port === port.port),
    );
    return others[0] ?? null;
  };

  const isEmpty = !loading && !error && nodes.length === 0;

  return (
    <WidgetCard
      title="Topology"
      expandHref={expandHref}
      contentClassName="overflow-hidden p-0"
      headerRight={
        <span className="text-xs text-muted-foreground">
          {nodes.length} components · {members.size} connections
        </span>
      }
    >
      {loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          Loading topology…
        </div>
      ) : error ? (
        <div className="flex h-full items-center justify-center text-sm text-destructive">
          {error}
        </div>
      ) : isEmpty ? (
        <div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted-foreground">
          No topology recorded in this trace.
        </div>
      ) : (
        <div className="flex h-full min-h-0">
          <div className="min-h-0 flex-1 overflow-auto p-2">
            <svg
              viewBox={`0 0 ${width} ${height}`}
              width="100%"
              height="100%"
              preserveAspectRatio="xMidYMid meet"
            >
                {edges.map((e, i) => {
                  const s = nodeById.get(e.source);
                  const t = nodeById.get(e.target);
                  if (!s || !t) return null;
                  const active = selected === e.source || selected === e.target;
                  return (
                    <g key={i}>
                      <line
                        x1={s.x}
                        y1={s.y}
                        x2={t.x}
                        y2={t.y}
                        stroke={active ? SELECTED_COLOR : "#cbd5e1"}
                        strokeWidth={active ? 2 : 1.5}
                      >
                        <title>{`${e.sourcePort} ↔ ${e.targetPort}  (${e.connection})`}</title>
                      </line>
                      <PortLabel
                        x={lerp(s.x, t.x, 0.28)}
                        y={lerp(s.y, t.y, 0.28)}
                        text={shortPort(e.sourcePort)}
                      />
                      <PortLabel
                        x={lerp(s.x, t.x, 0.72)}
                        y={lerp(s.y, t.y, 0.72)}
                        text={shortPort(e.targetPort)}
                      />
                    </g>
                  );
                })}

                {nodes.map((n) => {
                  const sel = n.id === selected;
                  return (
                    <g
                      key={n.id}
                      style={{ cursor: "pointer" }}
                      onClick={() => setSelected(sel ? null : n.id)}
                    >
                      <circle
                        cx={n.x}
                        cy={n.y}
                        r={sel ? NODE_R + 2 : NODE_R}
                        fill={sel ? SELECTED_COLOR : NODE_COLOR}
                        stroke="#fff"
                        strokeWidth={1.5}
                      />
                      <text
                        x={n.x + NODE_R + 5}
                        y={n.y + 4}
                        fontSize={11}
                        fontWeight={sel ? 600 : 400}
                        fill="#334155"
                        paintOrder="stroke"
                        stroke="#fff"
                        strokeWidth={3}
                        strokeLinejoin="round"
                      >
                        {n.id}
                      </text>
                    </g>
                  );
                })}
              </svg>
            </div>

            <aside className="w-72 shrink-0 overflow-auto border-l p-3">
              {selected ? (
                <div className="flex flex-col gap-3">
                  <div>
                    <div className="break-all text-sm font-semibold">
                      {selected}
                    </div>
                    <div className="font-mono text-xs text-muted-foreground">
                      {selectedComponent?.type || "no spec type"}
                    </div>
                  </div>

                  <div>
                    <div className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      Ports
                    </div>
                    <ul className="flex flex-col gap-1.5 text-xs">
                      {selectedPorts.map((p) => {
                        const peer = peerOf(p);
                        return (
                          <li key={p.port} className="flex flex-col">
                            <span className="font-mono font-medium">
                              {shortPort(p.port)}
                            </span>
                            <span className="text-muted-foreground">
                              {peer
                                ? `→ ${peer.component}.${shortPort(peer.port)}`
                                : "unconnected"}
                            </span>
                          </li>
                        );
                      })}
                      {selectedPorts.length === 0 ? (
                        <li className="text-muted-foreground">No ports.</li>
                      ) : null}
                    </ul>
                  </div>

                  {selectedComponent?.spec ? (
                    <div>
                      <div className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        Spec
                      </div>
                      <pre className="overflow-auto rounded bg-muted p-2 text-xs leading-relaxed">
                        {JSON.stringify(selectedComponent.spec, null, 2)}
                      </pre>
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className="text-sm text-muted-foreground">
                  Click a component to see its ports and spec.
                </div>
              )}
            </aside>
          </div>
        )}
    </WidgetCard>
  );
}

function PortLabel({ x, y, text }: { x: number; y: number; text: string }) {
  return (
    <text
      x={x}
      y={y}
      fontSize={9}
      fill="#64748b"
      textAnchor="middle"
      paintOrder="stroke"
      stroke="#fff"
      strokeWidth={2.5}
      strokeLinejoin="round"
    >
      {text}
    </text>
  );
}
