// Types for the main-page overview widgets, mirroring the Go structs served by
// /api/sim_info and /api/topology.

/** One key/value row of the simulation's exec_info table. */
export interface SimInfoEntry {
  property: string;
  value: string;
}

/** A component with its recorded spec. `spec` is null when none was recorded. */
export interface TopologyComponent {
  name: string;
  type: string;
  spec: Record<string, unknown> | null;
}

/** A single port and the connection it is plugged into ("" when unconnected). */
export interface TopologyPort {
  component: string;
  port: string;
  connection: string;
}

/** The static structure of a simulation: components and the full port inventory. */
export interface Topology {
  components: TopologyComponent[];
  ports: TopologyPort[];
}

/** A component ranked by how long its tasks spent blocked (virtual time). */
export interface BlockedComponent {
  component: string;
  blocked_time: number;
}

/** One directory entry of the recorded source tree. */
export interface CodeEntry {
  name: string;
  is_dir: boolean;
  size?: number;
}

/** Listing of a directory of recorded source ({@link Topology}-style empty-safe). */
export interface CodeLsResponse {
  path: string;
  roots: string[];
  entries: CodeEntry[];
}

/** Contents of a recorded source file. */
export interface CodeReadResponse {
  path: string;
  content: string;
  lines: number;
}
