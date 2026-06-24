import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { LineChart, ListTree } from "lucide-react";
import { select, zoom, zoomIdentity } from "d3";
import WidgetCard from "./WidgetCard";
import { Button } from "./ui/button";
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
  // Port identity at each end (network layout only), for per-port highlighting.
  aPort?: string;
  bPort?: string;
  // The direction each end leaves its box: +1 down (bottom port), -1 up (top
  // port), 0 a hub center. Used to bow the edge out of the row.
  adir?: number;
  bdir?: number;
}

// A ConnNode is the glyph for a connection that joins three or more ports: the
// connection is materialized as its own hub so the shared medium shows as a
// star, not a misleading clique of pairwise edges.
interface ConnNode {
  id: string;
  connection: string;
  short: string;
  cx: number;
  cy: number;
}

interface Layout {
  boxes: CompBox[];
  connNodes: ConnNode[];
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

// buildNetworkLayout lays out a topology that does not follow the ".Top"
// hierarchy convention (e.g. a Network-on-Chip: switches and endpoints wired by
// NetworkPort/Port[i]). It uses the actual connection graph: BFS-layering nodes
// by hop distance from a hub root, ordering each layer by the barycenter of its
// parents to reduce crossings. A connection that joins 3+ ports is materialized
// as its own hub node so the shared medium renders as a star instead of a
// clique. Each port is placed on the top or bottom edge depending on whether
// its peer sits above or below it in the layering.
function buildNetworkLayout(topology: Topology): Layout {
  const compByName = new Map(topology.components.map((c) => [c.name, c]));

  const compIds: string[] = [];
  const seen = new Set<string>();
  const addId = (id: string) => {
    if (!seen.has(id)) {
      seen.add(id);
      compIds.push(id);
    }
  };
  topology.components.forEach((c) => addId(c.name));
  topology.ports.forEach((p) => addId(p.component));

  const portsOf = new Map<string, { port: string; connection: string }[]>();
  compIds.forEach((id) => portsOf.set(id, []));
  topology.ports.forEach((p) => {
    portsOf.get(p.component)?.push({ port: p.port, connection: p.connection });
  });

  const members = membersByConnection(topology);
  const compSet = new Set(compIds);
  const connNodeId = (conn: string) => `conn:${conn}`;

  // A connection whose name is itself a component is a direct connection (or
  // similar) registered as a component: that component is its hub, so members
  // wire to it. A connection that is not a component but still joins 3+ distinct
  // components gets a synthetic hub node so the shared medium shows as a star.
  const connIsComp = (conn: string) => compSet.has(conn);
  const compsOf = (conn: string) =>
    Array.from(new Set((members.get(conn) ?? []).map((m) => m.component)));
  const synthHubs = new Set<string>();
  members.forEach((_list, conn) => {
    if (!connIsComp(conn) && compsOf(conn).length >= 3) synthHubs.add(conn);
  });

  const nodeIds = [...compIds, ...[...synthHubs].map(connNodeId)];

  // Undirected adjacency over the layout graph.
  const adj = new Map<string, Set<string>>();
  nodeIds.forEach((id) => adj.set(id, new Set()));
  const link = (a: string, b: string) => {
    if (a !== b) {
      adj.get(a)?.add(b);
      adj.get(b)?.add(a);
    }
  };
  members.forEach((_list, conn) => {
    const comps = compsOf(conn);
    if (connIsComp(conn)) {
      comps.forEach((c) => link(c, conn));
    } else if (comps.length === 2) {
      link(comps[0], comps[1]);
    } else if (comps.length >= 3) {
      const hub = connNodeId(conn);
      comps.forEach((c) => link(c, hub));
    }
  });

  const degree = (id: string) => adj.get(id)?.size ?? 0;
  const byDegreeThenName = (a: string, b: string) =>
    degree(b) - degree(a) || (a < b ? -1 : 1);

  // BFS depth from the highest-degree hub, repeating for any disconnected
  // remainder so every node is placed.
  const depth = new Map<string, number>();
  const queue: string[] = [];
  const runBFS = () => {
    let head = 0;
    while (head < queue.length) {
      const id = queue[head++];
      const d = depth.get(id)!;
      adj.get(id)?.forEach((nb) => {
        if (!depth.has(nb)) {
          depth.set(nb, d + 1);
          queue.push(nb);
        }
      });
    }
  };
  const componentRoots = compIds.slice().sort(byDegreeThenName);
  for (const r of [...componentRoots, ...nodeIds]) {
    if (!depth.has(r)) {
      depth.set(r, 0);
      queue.push(r);
      runBFS();
    }
  }

  // Group by depth, then order each layer by the barycenter of already-placed
  // neighbors one level up (a single Sugiyama-style pass).
  const byDepth = new Map<number, string[]>();
  nodeIds.forEach((id) => {
    const d = depth.get(id) ?? 0;
    const arr = byDepth.get(d) ?? [];
    arr.push(id);
    byDepth.set(d, arr);
  });
  const maxDepth = byDepth.size === 0 ? 0 : Math.max(...byDepth.keys());

  const order = new Map<string, number>();
  for (let d = 0; d <= maxDepth; d++) {
    const arr = (byDepth.get(d) ?? []).slice();
    const bary = (id: string): number => {
      const ups = [...(adj.get(id) ?? [])].filter(
        (nb) => (depth.get(nb) ?? -1) === d - 1 && order.has(nb),
      );
      if (ups.length === 0) return Number.POSITIVE_INFINITY;
      return ups.reduce((s, nb) => s + (order.get(nb) ?? 0), 0) / ups.length;
    };
    arr.sort((a, b) => {
      const ba = bary(a);
      const bb = bary(b);
      if (ba !== bb) return ba - bb;
      return a < b ? -1 : 1;
    });
    arr.forEach((id, i) => order.set(id, i));
    byDepth.set(d, arr);
  }

  // Positions, with each layer centered against the widest one.
  const widest = Math.max(1, ...[...byDepth.values()].map((a) => a.length));
  const pos = new Map<string, { cx: number; cy: number }>();
  for (let d = 0; d <= maxDepth; d++) {
    const arr = byDepth.get(d) ?? [];
    const offset = (widest - arr.length) / 2;
    arr.forEach((id, i) => {
      pos.set(id, {
        cx: MARGIN + (i + offset) * COL_GAP + 140,
        cy: MARGIN + d * ROW_GAP + 60,
      });
    });
  }

  const peerIdOf = (comp: string, port: string, conn: string): string | null => {
    if (!conn) return null;
    if (connIsComp(conn)) return conn === comp ? null : conn;
    if (synthHubs.has(conn)) return connNodeId(conn);
    const list = members.get(conn) ?? [];
    const other = list.find((m) => !(m.component === comp && m.port === port));
    return other ? other.component : null;
  };

  // Component boxes, with each port assigned to the top or bottom edge by
  // whether its peer sits above or below it.
  const portPos = new Map<string, { x: number; y: number }>();
  const boxes: CompBox[] = compIds.map((id) => {
    const p = pos.get(id) ?? { cx: MARGIN, cy: MARGIN };
    const myDepth = depth.get(id) ?? 0;
    const raw = portsOf.get(id) ?? [];
    const enriched = raw.map((pp) => {
      const peer = peerIdOf(id, pp.port, pp.connection);
      const peerDepth = peer ? depth.get(peer) ?? myDepth : myDepth;
      return {
        ...pp,
        short: shortPort(pp.port),
        w: portWidth(shortPort(pp.port)),
        up: peerDepth < myDepth,
      };
    });
    const top = enriched.filter((e) => e.up);
    const bottom = enriched.filter((e) => !e.up);

    const nameW = id.length * NAME_CW + RECT_PAD * 2;
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
      let x = p.cx - total / 2;
      return row.map((pp) => {
        const gx = x + pp.w / 2;
        x += pp.w + PORT_GAP;
        portPos.set(`${id}|${pp.port}`, { x: gx, y: edgeY });
        return {
          port: pp.port,
          short: pp.short,
          connection: pp.connection,
          cx: gx,
          cy: edgeY,
          w: pp.w,
        };
      });
    };

    const ports = [
      ...layRow(top, p.cy - RECT_H / 2),
      ...layRow(bottom, p.cy + RECT_H / 2),
    ];
    return { id, component: compByName.get(id) ?? null, cx: p.cx, cy: p.cy, rectW, ports };
  });

  const connNodes: ConnNode[] = [...synthHubs].map((conn) => {
    const p = pos.get(connNodeId(conn)) ?? { cx: MARGIN, cy: MARGIN };
    return { id: connNodeId(conn), connection: conn, short: shortPort(conn), cx: p.cx, cy: p.cy };
  });

  // Edges. A connection that is a component is drawn as spokes from each member
  // port to that component's box; a plain 2-member connection is a port-to-port
  // line; a non-component 3+-member connection spokes to its synthetic hub.
  const edges: Edge[] = [];
  const dirOf = (comp: string, y: number) => {
    const c = pos.get(comp);
    return c ? (y > c.cy ? 1 : -1) : 0;
  };
  const spokeToHub = (list: { component: string; port: string }[], conn: string, hub: { cx: number; cy: number }, hubId: string) => {
    list.forEach((m) => {
      if (m.component === hubId) return;
      const pm = portPos.get(`${m.component}|${m.port}`);
      if (pm) {
        edges.push({
          connection: conn,
          ax: pm.x,
          ay: pm.y,
          bx: hub.cx,
          by: hub.cy,
          a: m.component,
          b: hubId,
          aPort: m.port,
          adir: dirOf(m.component, pm.y),
          bdir: 0,
        });
      }
    });
  };
  members.forEach((list, conn) => {
    const comps = compsOf(conn);
    if (connIsComp(conn)) {
      const hub = pos.get(conn);
      if (hub) spokeToHub(list, conn, hub, conn);
    } else if (comps.length === 2 && list.length >= 2) {
      const pa = portPos.get(`${list[0].component}|${list[0].port}`);
      const pb = portPos.get(`${list[1].component}|${list[1].port}`);
      if (pa && pb) {
        edges.push({
          connection: conn,
          ax: pa.x,
          ay: pa.y,
          bx: pb.x,
          by: pb.y,
          a: list[0].component,
          b: list[1].component,
          aPort: list[0].port,
          bPort: list[1].port,
          adir: dirOf(list[0].component, pa.y),
          bdir: dirOf(list[1].component, pb.y),
        });
      }
    } else if (comps.length >= 3) {
      const hub = pos.get(connNodeId(conn));
      if (hub) spokeToHub(list, conn, hub, connNodeId(conn));
    }
  });

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
  connNodes.forEach((c) => {
    minX = Math.min(minX, c.cx - 24);
    maxX = Math.max(maxX, c.cx + 24);
    minY = Math.min(minY, c.cy - 24);
    maxY = Math.max(maxY, c.cy + 24);
  });
  if (!Number.isFinite(minX)) {
    minX = 0;
    minY = 0;
    maxX = 1;
    maxY = 1;
  }

  return {
    boxes,
    connNodes,
    edges,
    minX: minX - MARGIN,
    minY: minY - MARGIN,
    width: maxX - minX + MARGIN * 2,
    height: maxY - minY + MARGIN * 2,
  };
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

  // No ".Top" tree at all (e.g. a network whose ports are NetworkPort/Port[i])
  // — fall back to a connection-graph layout instead of dumping every node into
  // a single row.
  if (parent.size === 0) {
    return buildNetworkLayout(topology);
  }

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
    connNodes: [],
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

// isEdgeActive decides whether an edge is highlighted for the current selection.
// A selected port lights up only its own edge (when the edge carries port
// identity); a selected component lights up all of its edges.
function isEdgeActive(e: Edge, selected: Selection): boolean {
  if (!selected) return false;
  if (
    selected.kind === "port" &&
    (e.aPort !== undefined || e.bPort !== undefined)
  ) {
    return (
      (e.a === selected.component && e.aPort === selected.port) ||
      (e.b === selected.component && e.bPort === selected.port)
    );
  }
  return selected.component === e.a || selected.component === e.b;
}

interface TopologyWidgetProps {
  expandHref?: string;
  bare?: boolean;
}

export default function TopologyWidget({ expandHref, bare }: TopologyWidgetProps) {
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
      bare={bare}
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
            {!selected ? (
              <div className="pointer-events-none absolute left-3 top-3 z-10 text-xs text-muted-foreground">
                Click a component for its spec, or a port for its connection.
              </div>
            ) : null}
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
                const active = isEdgeActive(e, selected);
                // Bow the edge out of its row: leave each port perpendicular to
                // the box edge so a long same-row link arcs through the gap
                // instead of lying flat across the boxes between its ends.
                const bow = Math.min(90, 22 + 0.16 * Math.abs(e.bx - e.ax));
                const c1y = e.ay + (e.adir ?? 0) * bow;
                const c2y = e.by + (e.bdir ?? 0) * bow;
                const d = `M ${e.ax},${e.ay} C ${e.ax},${c1y} ${e.bx},${c2y} ${e.bx},${e.by}`;
                return (
                  <path
                    key={i}
                    d={d}
                    fill="none"
                    stroke={active ? SELECTED_COLOR : "#cbd5e1"}
                    strokeWidth={active ? 2.4 : 1.4}
                    strokeOpacity={selected && !active ? 0.2 : 1}
                  >
                    <title>{e.connection}</title>
                  </path>
                );
              })}

              {layout.connNodes.map((c) => (
                <g key={c.id}>
                  <circle
                    cx={c.cx}
                    cy={c.cy}
                    r={9}
                    fill="#ede9fe"
                    stroke="#7c3aed"
                    strokeWidth={1.75}
                  >
                    <title>{c.connection}</title>
                  </circle>
                  <text
                    x={c.cx}
                    y={c.cy - 14}
                    textAnchor="middle"
                    fontSize={9}
                    fill="#6d28d9"
                    pointerEvents="none"
                  >
                    {c.short}
                  </text>
                </g>
              ))}

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

              {selected?.kind === "port"
                ? layout.edges
                    .filter((e) => isEdgeActive(e, selected))
                    .map((e, i) => {
                      // Ring the port at the far end of the selected port's edge
                      // so its destination is unmistakable even on a long link.
                      const peerIsA = !(
                        e.a === selected.component &&
                        e.aPort === selected.port
                      );
                      const px = peerIsA ? e.ax : e.bx;
                      const py = peerIsA ? e.ay : e.by;
                      return (
                        <circle
                          key={`peer-${i}`}
                          cx={px}
                          cy={py}
                          r={8}
                          fill="none"
                          stroke={SELECTED_COLOR}
                          strokeWidth={2}
                          pointerEvents="none"
                        />
                      );
                    })
                : null}
              </g>
            </svg>
          </div>

          {selected ? (
            <aside className="w-96 shrink-0 overflow-auto border-l p-3">
              {selected.kind === "component" ? (
                <ComponentDetail
                  component={selComponent}
                  name={selected.component}
                />
              ) : portInfo ? (
                <PortDetail
                  component={selected.component}
                  port={selected.port}
                  peer={portInfo}
                />
              ) : null}
            </aside>
          ) : null}
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
      <div className="flex flex-col gap-1.5">
        <Button asChild variant="outline" size="sm" className="justify-start">
          <Link to={`/dashboard?widget=${encodeURIComponent(name)}`}>
            <LineChart />
            View component metrics
          </Link>
        </Button>
        <Button asChild variant="outline" size="sm" className="justify-start">
          <Link to={`/component?name=${encodeURIComponent(name)}`}>
            <ListTree />
            View component tasks
          </Link>
        </Button>
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
            <td className="whitespace-nowrap py-1 pr-3 font-mono text-muted-foreground">{key}</td>
            <td className="break-all py-1 text-left font-mono">
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
