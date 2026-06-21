import { useEffect, useMemo, useRef, useState } from "react";
import { select, zoom, zoomIdentity } from "d3";
import WidgetCard from "./WidgetCard";
import { useTopology } from "../hooks/useTopology";
import type { Topology, TopologyComponent } from "../types/overview";

const EMPTY_TOPOLOGY: Topology = { components: [], ports: [] };

// Glyph geometry. Components are rounded rectangles; ports are horizontally
// stretched hexagons sitting on the component's edges.
const RECT_H = 48;
const RECT_PAD = 18;
const RECT_SIDE_PAD = 22; // breathing room between the port row and the box ends
const NAME_CW = 7.4;
const PORT_H = 19;
const PORT_PAD = 14;
const PORT_CW = 5.6;
const PORT_GAP = 8;
const COL_GAP = 300;
const ROW_GAP = 170;
const MARGIN = 30;

const NODE_COLOR = "#2c7bb6";
const SELECTED_COLOR = "#f97316";

interface PortGlyph {
  port: string;
  short: string;
  connection: string;
  cx: number;
  cy: number;
  w: number;
}

interface CompBox {
  id: string;
  component: TopologyComponent | null;
  cx: number;
  cy: number;
  rectW: number;
  ports: PortGlyph[];
}

interface Edge {
  connection: string;
  ax: number;
  ay: number;
  bx: number;
  by: number;
  a: string; // component a
  b: string; // component b
}

interface Layout {
  boxes: CompBox[];
  edges: Edge[];
  minX: number;
  minY: number;
  width: number;
  height: number;
}

function shortPort(full: string): string {
  const dot = full.lastIndexOf(".");
  return dot >= 0 ? full.slice(dot + 1) : full;
}

function isTopPort(port: string): boolean {
  return shortPort(port).toLowerCase() === "top";
}

function portWidth(short: string): number {
  return Math.max(26, short.length * PORT_CW + PORT_PAD);
}

function rowWidth(ports: { w: number }[]): number {
  if (ports.length === 0) return 0;
  return (
    ports.reduce((s, p) => s + p.w, 0) + (ports.length - 1) * PORT_GAP
  );
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

// buildLayout produces a deterministic layered block diagram: a tidy tree of
// component rectangles (oriented via the ".Top" port convention) with each
// component's ports placed as hexagons on its top and bottom edges, and edges
// wired between the ports that share a connection.
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

  const portsOf = new Map<string, { port: string; connection: string }[]>();
  ids.forEach((id) => portsOf.set(id, []));
  topology.ports.forEach((p) => {
    portsOf.get(p.component)?.push({ port: p.port, connection: p.connection });
  });

  // Parent/child orientation from the ".Top" convention.
  const children = new Map<string, string[]>();
  ids.forEach((id) => children.set(id, []));
  const parent = new Map<string, string>();
  const members = membersByConnection(topology);
  members.forEach((list) => {
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const a = list[i];
        const b = list[j];
        let p: string | null = null;
        let c: string | null = null;
        if (isTopPort(b.port) && !isTopPort(a.port)) {
          p = a.component;
          c = b.component;
        } else if (isTopPort(a.port) && !isTopPort(b.port)) {
          p = b.component;
          c = a.component;
        }
        if (p && c && p !== c && !parent.has(c)) {
          parent.set(c, p);
          children.get(p)!.push(c);
        }
      }
    }
  });

  let roots = ids.filter((id) => !parent.has(id));
  if (roots.length === 0 && ids.length > 0) roots = [ids[0]];

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

  // Build the component boxes with their port glyphs.
  const portPos = new Map<string, { x: number; y: number }>();
  const boxes: CompBox[] = ids.map((id) => {
    const cx = MARGIN + (order.get(id) ?? 0) * COL_GAP + 140;
    const cy = MARGIN + (depth.get(id) ?? 0) * ROW_GAP + 60;

    const raw = portsOf.get(id) ?? [];
    const top = raw
      .filter((p) => isTopPort(p.port))
      .map((p) => ({ ...p, short: shortPort(p.port), w: portWidth(shortPort(p.port)) }));
    const bottom = raw
      .filter((p) => !isTopPort(p.port))
      .map((p) => ({ ...p, short: shortPort(p.port), w: portWidth(shortPort(p.port)) }));

    const nameW = id.length * NAME_CW + RECT_PAD * 2;
    // The box must contain its widest port row so every port sits on its edge
    // rather than floating outside it.
    const rectW = Math.max(
      nameW,
      rowWidth(top) + RECT_SIDE_PAD * 2,
      rowWidth(bottom) + RECT_SIDE_PAD * 2,
      90,
    );

    const layRow = (
      row: { port: string; short: string; w: number; connection: string }[],
      edgeY: number,
    ): PortGlyph[] => {
      const total = rowWidth(row);
      let x = cx - total / 2;
      return row.map((p) => {
        const gx = x + p.w / 2;
        x += p.w + PORT_GAP;
        portPos.set(`${id}|${p.port}`, { x: gx, y: edgeY });
        return {
          port: p.port,
          short: p.short,
          connection: p.connection,
          cx: gx,
          cy: edgeY,
          w: p.w,
        };
      });
    };

    const ports = [
      ...layRow(top, cy - RECT_H / 2),
      ...layRow(bottom, cy + RECT_H / 2),
    ];

    return { id, component: compByName.get(id) ?? null, cx, cy, rectW, ports };
  });

  const edges: Edge[] = [];
  members.forEach((list, connection) => {
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const pa = portPos.get(`${list[i].component}|${list[i].port}`);
        const pb = portPos.get(`${list[j].component}|${list[j].port}`);
        if (!pa || !pb) continue;
        edges.push({
          connection,
          ax: pa.x,
          ay: pa.y,
          bx: pb.x,
          by: pb.y,
          a: list[i].component,
          b: list[j].component,
        });
      }
    }
  });

  // Bounding box (include glyph extents).
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  boxes.forEach((b) => {
    const halfW = Math.max(b.rectW, rowWidth(b.ports)) / 2 + 4;
    minX = Math.min(minX, b.cx - halfW);
    maxX = Math.max(maxX, b.cx + halfW);
    minY = Math.min(minY, b.cy - RECT_H / 2 - PORT_H);
    maxY = Math.max(maxY, b.cy + RECT_H / 2 + PORT_H);
  });
  if (!Number.isFinite(minX)) {
    minX = 0;
    minY = 0;
    maxX = 1;
    maxY = 1;
  }

  return {
    boxes,
    edges,
    minX: minX - MARGIN,
    minY: minY - MARGIN,
    width: maxX - minX + MARGIN * 2,
    height: maxY - minY + MARGIN * 2,
  };
}

function hexPoints(cx: number, cy: number, w: number, h: number): string {
  const dx = w / 2;
  const dy = h / 2;
  const cut = dy; // pointed left/right ends -> horizontally stretched hexagon
  return [
    [cx - dx, cy],
    [cx - dx + cut, cy - dy],
    [cx + dx - cut, cy - dy],
    [cx + dx, cy],
    [cx + dx - cut, cy + dy],
    [cx - dx + cut, cy + dy],
  ]
    .map((p) => p.join(","))
    .join(" ");
}

type Selection =
  | { kind: "component"; component: string }
  | { kind: "port"; component: string; port: string }
  | null;

interface TopologyWidgetProps {
  expandHref?: string;
}

export default function TopologyWidget({ expandHref }: TopologyWidgetProps) {
  const { data, loading, error } = useTopology();
  const topology = data ?? EMPTY_TOPOLOGY;
  const [selected, setSelected] = useState<Selection>(null);

  const layout = useMemo(() => buildLayout(topology), [topology]);
  const members = useMemo(() => membersByConnection(topology), [topology]);

  const svgRef = useRef<SVGSVGElement | null>(null);
  const viewportRef = useRef<SVGGElement | null>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const zoomRef = useRef<any>(null);

  const ready = !loading && !error && layout.boxes.length > 0;

  // Wire up wheel-zoom (toward the cursor) and drag-pan via d3-zoom, which also
  // preserves clicks (a small movement still selects). Re-runs when the svg
  // (re)mounts after data loads.
  useEffect(() => {
    const svg = svgRef.current;
    const viewport = viewportRef.current;
    if (!svg || !viewport) return undefined;

    const behavior = zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 8])
      .clickDistance(4)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .on("zoom", (event: any) => {
        viewport.setAttribute("transform", event.transform.toString());
      });
    zoomRef.current = behavior;

    const sel = select(svg);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    sel.call(behavior as any).on("dblclick.zoom", null);

    return () => {
      sel.on(".zoom", null);
    };
  }, [ready]);

  // Reset the view whenever the topology changes.
  useEffect(() => {
    const svg = svgRef.current;
    if (svg && zoomRef.current) {
      select(svg).call(zoomRef.current.transform, zoomIdentity);
    }
  }, [layout]);

  const resetView = () => {
    const svg = svgRef.current;
    if (svg && zoomRef.current) {
      select(svg).call(zoomRef.current.transform, zoomIdentity);
    }
  };

  const selComponent =
    selected?.kind === "component" || selected?.kind === "port"
      ? topology.components.find((c) => c.name === selected.component) ?? null
      : null;

  const peerOf = (component: string, port: string, connection: string) => {
    if (!connection) return null;
    const mem = members.get(connection) ?? [];
    return (
      mem.find((x) => !(x.component === component && x.port === port)) ?? null
    );
  };

  let portInfo: {
    conn: string;
    peer: { component: string; port: string } | null;
  } | null = null;
  if (selected?.kind === "port") {
    const conn =
      topology.ports.find(
        (p) => p.component === selected.component && p.port === selected.port,
      )?.connection ?? "";
    portInfo = { conn, peer: peerOf(selected.component, selected.port, conn) };
  }

  const isEmpty = !loading && !error && layout.boxes.length === 0;

  return (
    <WidgetCard
      title="Topology"
      expandHref={expandHref}
      contentClassName="overflow-hidden p-0"
      headerRight={
        <span className="text-xs text-muted-foreground">
          {layout.boxes.length} components · {members.size} connections
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
          <div className="relative min-h-0 flex-1 overflow-hidden p-2">
            <button
              type="button"
              onClick={resetView}
              className="absolute right-3 top-3 z-10 rounded border bg-white/90 px-2 py-0.5 text-xs text-muted-foreground shadow-sm hover:text-foreground"
              title="Reset view"
            >
              Reset
            </button>
            <svg
              ref={svgRef}
              viewBox={`${layout.minX} ${layout.minY} ${layout.width} ${layout.height}`}
              width="100%"
              height="100%"
              preserveAspectRatio="xMidYMid meet"
              style={{ cursor: "grab", touchAction: "none" }}
            >
              <g ref={viewportRef}>
              {layout.edges.map((e, i) => {
                const active =
                  selected?.component === e.a || selected?.component === e.b;
                return (
                  <line
                    key={i}
                    x1={e.ax}
                    y1={e.ay}
                    x2={e.bx}
                    y2={e.by}
                    stroke={active ? SELECTED_COLOR : "#cbd5e1"}
                    strokeWidth={active ? 2 : 1.4}
                  >
                    <title>{e.connection}</title>
                  </line>
                );
              })}

              {layout.boxes.map((b) => {
                const rectSelected =
                  selected?.kind === "component" && selected.component === b.id;
                return (
                  <g key={b.id}>
                    <rect
                      x={b.cx - b.rectW / 2}
                      y={b.cy - RECT_H / 2}
                      width={b.rectW}
                      height={RECT_H}
                      rx={11}
                      ry={11}
                      fill="#fff"
                      stroke={rectSelected ? SELECTED_COLOR : NODE_COLOR}
                      strokeWidth={rectSelected ? 2.5 : 1.75}
                      style={{ cursor: "pointer" }}
                      onClick={() =>
                        setSelected(
                          rectSelected
                            ? null
                            : { kind: "component", component: b.id },
                        )
                      }
                    />
                    <text
                      x={b.cx}
                      y={b.cy + 5}
                      textAnchor="middle"
                      fontSize={13}
                      fontWeight={600}
                      fill="#1e293b"
                      pointerEvents="none"
                    >
                      {b.id}
                    </text>

                    {b.ports.map((p) => {
                      const portSelected =
                        selected?.kind === "port" &&
                        selected.component === b.id &&
                        selected.port === p.port;
                      const connected = p.connection !== "";
                      return (
                        <g
                          key={p.port}
                          style={{ cursor: "pointer" }}
                          onClick={() =>
                            setSelected(
                              portSelected
                                ? null
                                : { kind: "port", component: b.id, port: p.port },
                            )
                          }
                        >
                          <polygon
                            points={hexPoints(p.cx, p.cy, p.w, PORT_H)}
                            fill={
                              portSelected
                                ? SELECTED_COLOR
                                : connected
                                  ? "#e2eef7"
                                  : "#f1f5f9"
                            }
                            stroke={portSelected ? SELECTED_COLOR : "#94a3b8"}
                            strokeWidth={1}
                          />
                          <text
                            x={p.cx}
                            y={p.cy + 3}
                            textAnchor="middle"
                            fontSize={9}
                            fill={portSelected ? "#fff" : "#475569"}
                            pointerEvents="none"
                          >
                            {p.short}
                          </text>
                        </g>
                      );
                    })}
                  </g>
                );
              })}
              </g>
            </svg>
          </div>

          <aside className="w-72 shrink-0 overflow-auto border-l p-3">
            {selected?.kind === "component" ? (
              <ComponentDetail component={selComponent} name={selected.component} />
            ) : selected?.kind === "port" && portInfo ? (
              <PortDetail
                component={selected.component}
                port={selected.port}
                peer={portInfo}
              />
            ) : (
              <div className="text-sm text-muted-foreground">
                Click a component for its spec, or a port for its connection.
              </div>
            )}
          </aside>
        </div>
      )}
    </WidgetCard>
  );
}

function ComponentDetail({
  component,
  name,
}: {
  component: TopologyComponent | null;
  name: string;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="break-all text-sm font-semibold">{name}</div>
      <div className="font-mono text-xs text-muted-foreground">
        {component?.type || "no spec type"}
      </div>
      {component?.spec ? (
        <SpecTable spec={component.spec} />
      ) : (
        <div className="text-sm text-muted-foreground">
          No spec recorded for this component.
        </div>
      )}
    </div>
  );
}

function SpecTable({ spec }: { spec: Record<string, unknown> }) {
  const entries = Object.entries(spec);
  if (entries.length === 0) {
    return <div className="text-sm text-muted-foreground">Empty spec.</div>;
  }
  return (
    <table className="w-full border-collapse text-xs">
      <tbody>
        {entries.map(([key, value]) => (
          <tr key={key} className="border-b border-border/60 align-top">
            <td className="py-1 pr-3 font-mono text-muted-foreground">{key}</td>
            <td className="break-all py-1 text-right font-mono">
              {typeof value === "object"
                ? JSON.stringify(value)
                : String(value)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function PortDetail({
  component,
  port,
  peer,
}: {
  component: string;
  port: string;
  peer: { conn: string; peer: { component: string; port: string } | null };
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="break-all text-sm font-semibold">{port}</div>
      <div className="text-xs text-muted-foreground">on {component}</div>
      <table className="w-full border-collapse text-xs">
        <tbody>
          <tr className="border-b border-border/60 align-top">
            <td className="py-1 pr-3 font-mono text-muted-foreground">connection</td>
            <td className="break-all py-1 text-right font-mono">
              {peer.conn || "—"}
            </td>
          </tr>
          <tr className="align-top">
            <td className="py-1 pr-3 font-mono text-muted-foreground">peer</td>
            <td className="break-all py-1 text-right font-mono">
              {peer.peer
                ? `${peer.peer.component}.${shortPort(peer.peer.port)}`
                : "unconnected"}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
