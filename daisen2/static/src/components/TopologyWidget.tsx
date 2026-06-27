import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { LineChart, ListTree } from "lucide-react";
import { select, zoom, zoomIdentity } from "d3";
import WidgetCard from "./WidgetCard";
import { Button } from "./ui/button";
import { useTopology } from "../hooks/useTopology";
import type {
  Topology,
  TopologyComponent,
  TopologyPort,
} from "../types/overview";

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
  // Hierarchical view only: the display label (id stays the unique node key),
  // the array size for a "Base[*] ×N" group, and whether clicking expands this
  // box (a domain/array) versus selects it (a leaf component for its spec).
  label?: string;
  count?: number;
  expandable?: boolean;
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

  const peerIdOf = (comp: string, port: string, conn: string): string | null => {
    if (!conn) return null;
    if (connIsComp(conn)) return conn === comp ? null : conn;
    if (synthHubs.has(conn)) return connNodeId(conn);
    const list = members.get(conn) ?? [];
    const other = list.find((m) => !(m.component === comp && m.port === port));
    return other ? other.component : null;
  };

  // Pre-compute each node's drawn width (a component box from its name + port
  // rows; a hub is a small circle) so the layout can space nodes by their real
  // width and wrap a too-wide layer into several rows instead of one long line.
  const nodeWidth = new Map<string, number>();
  compIds.forEach((id) => {
    const myDepth = depth.get(id) ?? 0;
    const top: { w: number }[] = [];
    const bottom: { w: number }[] = [];
    (portsOf.get(id) ?? []).forEach((pp) => {
      const peer = peerIdOf(id, pp.port, pp.connection);
      const peerDepth = peer ? depth.get(peer) ?? myDepth : myDepth;
      const w = portWidth(shortPort(pp.port));
      (peerDepth < myDepth ? top : bottom).push({ w });
    });
    const nameW = id.length * NAME_CW + RECT_PAD * 2;
    nodeWidth.set(
      id,
      Math.max(
        nameW,
        rowWidth(top) + RECT_SIDE_PAD * 2,
        rowWidth(bottom) + RECT_SIDE_PAD * 2,
        90,
      ),
    );
  });
  synthHubs.forEach((conn) => nodeWidth.set(connNodeId(conn), 30));
  const widthOf = (id: string) => nodeWidth.get(id) ?? 90;

  // Lay each depth left-to-right by cumulative width, wrapping into sub-rows once
  // the running width exceeds a target — so a wide layer forms a compact grid
  // rather than one long line, and boxes never overlap. Rows are centred on
  // x = 0; absolute position is normalised by the bounding box below.
  const HGAP = 46;
  const MAX_ROW_W = 2200;
  const pos = new Map<string, { cx: number; cy: number }>();
  let yCursor = 0;
  for (let d = 0; d <= maxDepth; d++) {
    const arr = byDepth.get(d) ?? [];
    const subRows: string[][] = [];
    let cur: string[] = [];
    let curW = 0;
    arr.forEach((id) => {
      const w = widthOf(id) + HGAP;
      if (cur.length && curW + w > MAX_ROW_W) {
        subRows.push(cur);
        cur = [];
        curW = 0;
      }
      cur.push(id);
      curW += w;
    });
    if (cur.length) subRows.push(cur);
    subRows.forEach((row) => {
      const totalW = row.reduce((s, id) => s + widthOf(id) + HGAP, -HGAP);
      let x = -totalW / 2;
      row.forEach((id) => {
        const w = widthOf(id);
        pos.set(id, { cx: x + w / 2, cy: yCursor });
        x += w + HGAP;
      });
      yCursor += ROW_GAP;
    });
    yCursor += 30;
  }

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

// groupKeyOf collapses array indices in a component name so a row of sibling
// components (GPU[1].CU[0..63]) shares one key. Every "[<digits>]" becomes
// "[*]", turning hundreds of identical leaves into a handful of groups.
function groupKeyOf(name: string): string {
  return name.replace(/\[\d+\]/g, "[*]");
}

// buildAggregatedLayout collapses each array of sibling components into a single
// group node (e.g. "GPU[1].CU[*] ×64") and lays the resulting handful of groups
// out as the same ".Top"-oriented tree buildLayout uses. It turns the
// hundreds-of-nodes "smear" into a readable block diagram while preserving the
// real structure; a single (non-array) component keeps its own name.
function buildAggregatedLayout(topology: Topology): Layout {
  const compByName = new Map<string, TopologyComponent>();
  topology.components.forEach((c) => compByName.set(c.name, c));

  const allNames = new Set<string>();
  topology.components.forEach((c) => allNames.add(c.name));
  topology.ports.forEach((p) => allNames.add(p.component));

  // Group every component by its index-collapsed name.
  const membersOf = new Map<string, string[]>();
  allNames.forEach((name) => {
    const key = groupKeyOf(name);
    const arr = membersOf.get(key) ?? [];
    arr.push(name);
    membersOf.set(key, arr);
  });
  const groupIds = [...membersOf.keys()];

  // Display label: a single component keeps its real name; an array shows its
  // size. The id (rendered on the box) is the label, unique per group.
  const labelOf = new Map<string, string>();
  groupIds.forEach((g) => {
    const m = membersOf.get(g) ?? [];
    labelOf.set(g, m.length === 1 ? m[0] : `${g} ×${m.length}`);
  });

  // Group-level parent/child from the ".Top" convention, plus the distinct group
  // pairs that share any connection (these become the edges).
  const parent = new Map<string, string>();
  const children = new Map<string, string[]>();
  groupIds.forEach((g) => children.set(g, []));
  const pairKey = (a: string, b: string) =>
    a < b ? `${a} ${b}` : `${b} ${a}`;
  const pairs = new Set<string>();

  const members = membersByConnection(topology);
  members.forEach((list) => {
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const a = list[i];
        const b = list[j];
        const ga = groupKeyOf(a.component);
        const gb = groupKeyOf(b.component);
        if (ga === gb) continue;
        pairs.add(pairKey(ga, gb));
        let p: string | null = null;
        let c: string | null = null;
        if (isTopPort(b.port) && !isTopPort(a.port)) {
          p = ga;
          c = gb;
        } else if (isTopPort(a.port) && !isTopPort(b.port)) {
          p = gb;
          c = ga;
        }
        if (p && c && p !== c && !parent.has(c)) {
          parent.set(c, p);
          children.get(p)!.push(c);
        }
      }
    }
  });

  // Tree layout over the groups (same scheme as buildLayout): leaves take
  // sequential columns, parents centre over their children, depth sets the row.
  let roots = groupIds.filter((g) => !parent.has(g));
  if (roots.length === 0 && groupIds.length > 0) roots = [groupIds[0]];
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
  groupIds.forEach((g) => {
    if (!visited.has(g)) place(g, 0);
  });

  const boxes: CompBox[] = groupIds.map((g) => {
    const label = labelOf.get(g) ?? g;
    const cx = MARGIN + (order.get(g) ?? 0) * COL_GAP + 140;
    const cy = MARGIN + (depth.get(g) ?? 0) * ROW_GAP + 60;
    const rectW = Math.max(label.length * NAME_CW + RECT_PAD * 2, 90);
    return {
      id: label,
      component: compByName.get((membersOf.get(g) ?? [])[0]) ?? null,
      cx,
      cy,
      rectW,
      ports: [],
    };
  });
  const boxByGroup = new Map<string, CompBox>();
  groupIds.forEach((g, i) => boxByGroup.set(g, boxes[i]));

  // One edge per group pair, routed from the upper box's bottom edge to the
  // lower box's top edge so the existing bowed-edge renderer arcs it cleanly.
  const edges: Edge[] = [];
  pairs.forEach((pk) => {
    const [g1, g2] = pk.split(" ");
    const b1 = boxByGroup.get(g1);
    const b2 = boxByGroup.get(g2);
    if (!b1 || !b2) return;
    const upper = b1.cy <= b2.cy ? b1 : b2;
    const lower = b1.cy <= b2.cy ? b2 : b1;
    edges.push({
      connection: `${upper.id} – ${lower.id}`,
      ax: upper.cx,
      ay: upper.cy + RECT_H / 2,
      adir: 1,
      bx: lower.cx,
      by: lower.cy - RECT_H / 2,
      bdir: -1,
      a: upper.id,
      b: lower.id,
    });
  });

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  boxes.forEach((b) => {
    const halfW = b.rectW / 2 + 4;
    minX = Math.min(minX, b.cx - halfW);
    maxX = Math.max(maxX, b.cx + halfW);
    minY = Math.min(minY, b.cy - RECT_H / 2);
    maxY = Math.max(maxY, b.cy + RECT_H / 2);
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

// segBase strips the array index from one path segment: "SA[0]" -> "SA".
function segBase(seg: string): string {
  return seg.replace(/\[\d+\]/g, "");
}

// defaultExpanded opens, initially, every top-level domain that has children, so
// the view starts at "GPU expanded one level" (its Shader Arrays and GPU-level
// units) rather than a single opaque GPU box. Keys are index-collapsed to match
// buildHierarchicalLayout (e.g. "GPU[1]" -> "GPU[*]").
function defaultExpanded(topology: Topology): Set<string> {
  const roots = new Set<string>();
  const hasChild = new Set<string>();
  const note = (n: string) => {
    const segs = n.split(".");
    const root = segs[0].replace(/\[\d+\]/g, "[*]");
    roots.add(root);
    if (segs.length > 1) hasChild.add(root);
  };
  topology.components.forEach((c) => note(c.name));
  topology.ports.forEach((p) => note(p.component));
  return new Set([...roots].filter((r) => hasChild.has(r)));
}

// buildHierarchicalLayout renders the components as a collapsible domain tree
// parsed from the dotted names (GPU -> Shader Array -> unit). Array siblings
// collapse to one "Base[*] xN" group per parent — so a Shader Array is a real
// level, not flattened into the leaves — and `expanded` holds the domain/array
// keys the user has opened. A connection internal to a collapsed domain (both
// ends land in the same box) is hidden; the rest become edges between the
// visible boxes they cross.
function buildHierarchicalLayout(
  topology: Topology,
  expanded: Set<string>,
): Layout {
  const compByName = new Map<string, TopologyComponent>();
  topology.components.forEach((c) => compByName.set(c.name, c));

  const allNames = new Set<string>();
  topology.components.forEach((c) => allNames.add(c.name));
  topology.ports.forEach((p) => allNames.add(p.component));

  // Collapse every array index ("[<n>]" -> "[*]") so a whole series shares one
  // key at each level, while keeping one key per dotted segment so the hierarchy
  // (GPU -> Shader Array -> unit) is preserved. An array is therefore always a
  // single "Base[*] ×N" block — never split into individual indices — and a
  // domain array (SA[*]) expands straight to its aggregated contents.
  const collapseKey = (prefix: string) => prefix.replace(/\[\d+\]/g, "[*]");

  // chainOf: the index-collapsed prefix at each depth, root -> leaf.
  const chainOf = (name: string): string[] => {
    const segs = name.split(".");
    const chain: string[] = [];
    let prefix = "";
    for (const seg of segs) {
      prefix = prefix === "" ? seg : `${prefix}.${seg}`;
      chain.push(collapseKey(prefix));
    }
    return chain;
  };

  // A collapsed key has children when some name extends it by another segment.
  const childKeys = new Map<string, Set<string>>();
  allNames.forEach((name) => {
    const chain = chainOf(name);
    for (let i = 1; i < chain.length; i++) {
      const s = childKeys.get(chain[i - 1]) ?? new Set<string>();
      s.add(chain[i]);
      childKeys.set(chain[i - 1], s);
    }
  });
  const hasChildren = (key: string) => (childKeys.get(key)?.size ?? 0) > 0;

  // Visible box for a component: the shallowest collapsed key the user has not
  // opened; the leaf key when every ancestor is open.
  const visibleKeyOf = (name: string): string => {
    const chain = chainOf(name);
    for (const k of chain) {
      if (!expanded.has(k)) return k;
    }
    return chain[chain.length - 1];
  };

  const boxMembers = new Map<string, string[]>();
  const visKeyOf = new Map<string, string>();
  allNames.forEach((name) => {
    const k = visibleKeyOf(name);
    visKeyOf.set(name, k);
    const arr = boxMembers.get(k) ?? [];
    arr.push(name);
    boxMembers.set(k, arr);
  });
  const visKeys = [...boxMembers.keys()];

  // Draw the visible boxes with real ports and port-to-port edges by reusing the
  // network layout: describe the boxes as a synthetic topology where each box is
  // a node, its external port *roles* are its ports, and each real connection
  // (index-collapsed) links box+role endpoints. A bus that touches 3+ boxes
  // becomes a hub node (star) instead of a many-to-many clique.
  const collapsedConnBoxes = new Map<string, Set<string>>();
  topology.ports.forEach((p) => {
    if (!p.connection) return;
    const box = visKeyOf.get(p.component);
    if (!box) return;
    const conn = collapseKey(p.connection);
    const s = collapsedConnBoxes.get(conn) ?? new Set<string>();
    s.add(box);
    collapsedConnBoxes.set(conn, s);
  });

  const synthComponents: TopologyComponent[] = visKeys.map((k) => ({
    name: k,
    type: "",
    spec: null,
  }));
  const seenSynthPort = new Set<string>();
  const synthPorts: TopologyPort[] = [];
  topology.ports.forEach((p) => {
    if (!p.connection) return;
    const box = visKeyOf.get(p.component);
    if (!box) return;
    const conn = collapseKey(p.connection);
    // Keep only connections that leave their box (cross between visible boxes);
    // wiring internal to a collapsed domain is not drawn.
    if ((collapsedConnBoxes.get(conn)?.size ?? 0) < 2) return;
    const portName = `${box}.${shortPort(p.port)}`;
    const sig = `${portName} ${conn}`;
    if (seenSynthPort.has(sig)) return;
    seenSynthPort.add(sig);
    synthPorts.push({ component: box, port: portName, connection: conn });
  });

  const layout = buildNetworkLayout({
    components: synthComponents,
    ports: synthPorts,
  });

  // Re-attach the hierarchical metadata (label / count / expandable / the
  // representative component for the spec & port panel) onto the laid-out boxes.
  const colSeg = (key: string) => key.split(".").pop() ?? key;
  layout.boxes = layout.boxes.map((b) => {
    const mem = boxMembers.get(b.id) ?? [];
    const d = b.id.split(".").length - 1;
    const idx = new Set<string>();
    mem.forEach((m) => idx.add(m.split(".")[d] ?? m));
    const count = idx.size;
    const repSeg = mem[0]?.split(".")[d] ?? colSeg(b.id);
    return {
      ...b,
      label: count > 1 ? `${colSeg(b.id)} ×${count}` : repSeg,
      count: count > 1 ? count : undefined,
      expandable: hasChildren(b.id),
      component: hasChildren(b.id) ? null : compByName.get(mem[0]) ?? null,
    };
  });

  return layout;
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
  | { kind: "port"; component: string; port: string; connection: string }
  | { kind: "connection"; connection: string }
  | null;

// isEdgeActive decides whether an edge is highlighted for the current selection.
// A selected port or connection lights up every edge of that connection — so a
// point-to-point link shows just itself, and a port (or hub) on a shared bus
// lights up the whole bus. A selected component lights up all of its own edges.
function isEdgeActive(e: Edge, selected: Selection): boolean {
  if (!selected) return false;
  if (selected.kind === "connection" || selected.kind === "port") {
    return e.connection === selected.connection;
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
  // The set of domain/array keys the user has opened. Default is fully collapsed
  // (empty): the top-level domains (e.g. a single GPU box) show, and the user
  // drills in from there.
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const exp = expanded;

  const layout = useMemo(
    () => buildHierarchicalLayout(topology, exp),
    [topology, exp],
  );

  // Clicking a domain/array box toggles it open/closed; clicking a leaf
  // component selects it for its spec.
  const handleBoxClick = (b: CompBox) => {
    if (b.expandable) {
      const next = new Set(exp);
      if (next.has(b.id)) next.delete(b.id);
      else next.add(b.id);
      setExpanded(next);
      return;
    }
    setSelected(
      selected?.kind === "component" && selected.component === b.id
        ? null
        : { kind: "component", component: b.id },
    );
  };
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
      // A wide trace (hundreds of components) is fit into the widget at a tiny
      // base scale, so the previous 8x cap could not magnify a single box enough
      // to read. Allow deep zoom-in (and a bit more zoom-out) so an individual
      // component is always reachable; the real fix is a less-wide layout.
      .scaleExtent([0.1, 100])
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

  // A selected leaf box carries the collapsed node key as its id; resolve its
  // representative component from the box, falling back to a real-name match.
  const selComponent =
    selected?.kind === "component" || selected?.kind === "port"
      ? layout.boxes.find((b) => b.id === selected.component)?.component ??
        topology.components.find((c) => c.name === selected.component) ??
        null
      : null;

  const peerOf = (component: string, port: string, connection: string) => {
    if (!connection) return null;
    const mem = members.get(connection) ?? [];
    return (
      mem.find((x) => !(x.component === component && x.port === port)) ?? null
    );
  };

  // For a selected port or hub: the connection it lights up, plus the boxes and
  // roles on that connection (for the side panel).
  const selConnection =
    selected?.kind === "connection"
      ? selected.connection
      : selected?.kind === "port"
        ? selected.connection
        : null;
  const labelByKey = new Map(layout.boxes.map((b) => [b.id, b.label ?? b.id]));
  const connEndpoints = selConnection
    ? Array.from(
        new Map(
          layout.edges
            .filter((e) => e.connection === selConnection)
            .flatMap((e) => {
              const out: [string, { box: string; role: string }][] = [];
              if (!e.a.startsWith("conn:") && e.aPort) {
                out.push([
                  e.aPort,
                  { box: labelByKey.get(e.a) ?? e.a, role: shortPort(e.aPort) },
                ]);
              }
              if (!e.b.startsWith("conn:") && e.bPort) {
                out.push([
                  e.bPort,
                  { box: labelByKey.get(e.b) ?? e.b, role: shortPort(e.bPort) },
                ]);
              }
              return out;
            }),
        ).values(),
      )
    : [];

  // The selected box's representative real component (its id is a collapsed key),
  // and that component's ports with what each is wired to — so the detail panel
  // can show port information for a leaf or a series block.
  const selBox =
    selected?.kind === "component"
      ? layout.boxes.find((b) => b.id === selected.component) ?? null
      : null;
  const selRealName = selBox?.component?.name ?? selComponent?.name ?? "";
  const selPorts =
    selected?.kind === "component" && selRealName
      ? topology.ports
          .filter((p) => p.component === selRealName)
          .map((p) => {
            const mem = members.get(p.connection) ?? [];
            const others = mem.filter(
              (x) => !(x.component === selRealName && x.port === p.port),
            );
            // A single peer is meaningful only for a point-to-point link; a
            // shared bus (many endpoints) is reported as the bus + its fan-out.
            return {
              port: p.port,
              short: shortPort(p.port),
              connection: p.connection,
              peer: others.length === 1 ? others[0] : null,
              fanout: mem.length,
            };
          })
      : [];

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
            <div className="absolute right-3 top-3 z-10 flex gap-2">
              <button
                type="button"
                onClick={() => setExpanded(new Set())}
                className="rounded border bg-white/90 px-2 py-0.5 text-xs text-muted-foreground shadow-sm hover:text-foreground"
                title="Collapse every domain to the top level"
              >
                Collapse all
              </button>
              <button
                type="button"
                onClick={resetView}
                className="rounded border bg-white/90 px-2 py-0.5 text-xs text-muted-foreground shadow-sm hover:text-foreground"
                title="Reset pan/zoom"
              >
                Reset
              </button>
            </div>
            {!selected ? (
              <div className="pointer-events-none absolute left-3 top-3 z-10 text-xs text-muted-foreground">
                Click a dashed domain (▸) to expand it; click a component for
                its spec.
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

              {layout.connNodes.map((c) => {
                const connSelected =
                  selected?.kind === "connection" &&
                  selected.connection === c.connection;
                return (
                  <g
                    key={c.id}
                    style={{ cursor: "pointer" }}
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelected(
                        connSelected
                          ? null
                          : { kind: "connection", connection: c.connection },
                      );
                    }}
                  >
                    <circle
                      cx={c.cx}
                      cy={c.cy}
                      r={connSelected ? 11 : 9}
                      fill={connSelected ? SELECTED_COLOR : "#ede9fe"}
                      stroke={connSelected ? SELECTED_COLOR : "#7c3aed"}
                      strokeWidth={connSelected ? 2.5 : 1.75}
                    >
                      <title>{c.connection}</title>
                    </circle>
                    <text
                      x={c.cx}
                      y={c.cy - 14}
                      textAnchor="middle"
                      fontSize={9}
                      fill={connSelected ? SELECTED_COLOR : "#6d28d9"}
                      pointerEvents="none"
                    >
                      {c.short}
                    </text>
                  </g>
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
                      fill={b.expandable ? "#f1f5f9" : "#fff"}
                      stroke={rectSelected ? SELECTED_COLOR : NODE_COLOR}
                      strokeWidth={rectSelected ? 2.5 : 1.75}
                      strokeDasharray={b.expandable ? "5 3" : undefined}
                      style={{ cursor: "pointer" }}
                      onClick={() => handleBoxClick(b)}
                    >
                      <title>
                        {b.expandable
                          ? `${b.id} — click to expand`
                          : b.id}
                      </title>
                    </rect>
                    <text
                      x={b.cx}
                      y={b.cy + 5}
                      textAnchor="middle"
                      fontSize={13}
                      fontWeight={600}
                      fill="#1e293b"
                      pointerEvents="none"
                    >
                      {b.expandable ? `${b.label ?? b.id} ▸` : b.label ?? b.id}
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
                          onClick={(e) => {
                            // Select this port's link: highlights its connection
                            // — just the one link, or the whole bus when the port
                            // wires to a hub.
                            e.stopPropagation();
                            const isSel =
                              selected?.kind === "port" &&
                              selected.component === b.id &&
                              selected.port === p.port;
                            setSelected(
                              isSel
                                ? null
                                : {
                                    kind: "port",
                                    component: b.id,
                                    port: p.port,
                                    connection: p.connection,
                                  },
                            );
                          }}
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
                  title={selBox?.label ?? (selRealName || selected.component)}
                  name={selRealName || selected.component}
                  count={selBox?.count}
                  ports={selPorts}
                />
              ) : selConnection ? (
                <LinkDetail
                  connection={selConnection}
                  fromBox={
                    selected.kind === "port"
                      ? labelByKey.get(selected.component) ?? selected.component
                      : null
                  }
                  fromRole={
                    selected.kind === "port" ? shortPort(selected.port) : null
                  }
                  endpoints={connEndpoints}
                />
              ) : null}
            </aside>
          ) : null}
        </div>
      )}
    </WidgetCard>
  );
}

function LinkDetail({
  connection,
  fromBox,
  fromRole,
  endpoints,
}: {
  connection: string;
  fromBox: string | null;
  fromRole: string | null;
  endpoints: { box: string; role: string }[];
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="break-all text-sm font-semibold">{connection}</div>
      {fromBox ? (
        <div className="text-xs text-muted-foreground">
          port <span className="font-mono">{fromRole}</span> on {fromBox}
        </div>
      ) : (
        <div className="text-xs text-muted-foreground">
          {endpoints.length > 2 ? "shared bus" : "link"}
        </div>
      )}
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Endpoints ({endpoints.length})
      </div>
      {endpoints.length === 0 ? (
        <div className="text-sm text-muted-foreground">No endpoints.</div>
      ) : (
        <table className="w-full border-collapse text-xs">
          <tbody>
            {endpoints.map((ep, i) => (
              <tr key={i} className="border-b border-border/60 align-top">
                <td className="break-all py-1 pr-3 font-mono">{ep.box}</td>
                <td className="whitespace-nowrap py-1 font-mono text-muted-foreground">
                  {ep.role}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function ComponentDetail({
  component,
  title,
  name,
  count,
  ports,
}: {
  component: TopologyComponent | null;
  title: string;
  name: string;
  count?: number;
  ports: {
    port: string;
    short: string;
    connection: string;
    peer: { component: string; port: string } | null;
    fanout: number;
  }[];
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="break-all text-sm font-semibold">{title}</div>
      {count && count > 1 ? (
        <div className="text-xs text-muted-foreground">
          Series of {count} — showing {name}
        </div>
      ) : null}
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

      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Ports ({ports.length})
      </div>
      {ports.length === 0 ? (
        <div className="text-sm text-muted-foreground">No ports recorded.</div>
      ) : (
        <table className="w-full border-collapse text-xs">
          <tbody>
            {ports.map((p) => (
              <tr key={p.port} className="border-b border-border/60 align-top">
                <td className="whitespace-nowrap py-1 pr-3 font-mono">
                  {p.short}
                </td>
                <td className="break-all py-1 text-left font-mono text-muted-foreground">
                  {!p.connection
                    ? "unconnected"
                    : p.peer
                      ? `→ ${p.peer.component} (${shortPort(p.peer.port)})`
                      : `${p.connection} · ${p.fanout} endpoints`}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

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
