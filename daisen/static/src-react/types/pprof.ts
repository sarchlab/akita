/** Types for pprof profiling data visualization. */

export interface PProfFunc {
  ID: number;
  Name: string;
  SystemName?: string;
  Filename?: string;
  StartLine?: number;
}

export interface PProfLocation {
  Line: Array<{ Function: PProfFunc }>;
}

export interface PProfSample {
  Location: PProfLocation[];
  Value: number[];
}

/** Raw data returned by GET /api/profile */
export interface PProfData {
  Function: PProfFunc[];
  Sample: PProfSample[];
}

export interface PProfEdge {
  time: number;
  timePercentage: number;
  caller: PProfNode;
  callee: PProfNode;
}

export interface PProfNode {
  index: number;
  func: PProfFunc;
  selfTime: number;
  time: number;
  edges: PProfEdge[];
  timePercentage: number;
  selfTimePercentage: number;
}

export interface PProfNetwork {
  nodes: PProfNode[];
  edges: Map<string, PProfEdge>;
}

/** Create a new empty PProfNode. */
export function createNode(func: PProfFunc): PProfNode {
  return {
    index: 0,
    func,
    selfTime: 0,
    time: 0,
    edges: [],
    timePercentage: 0,
    selfTimePercentage: 0,
  };
}

/** Create a new empty PProfNetwork. */
export function createNetwork(): PProfNetwork {
  return { nodes: [], edges: new Map() };
}

/** Get or create an edge between caller and callee. */
export function getOrCreateEdge(
  network: PProfNetwork,
  callerIndex: number,
  calleeIndex: number,
): PProfEdge {
  const key = `${callerIndex}-${calleeIndex}`;
  let edge = network.edges.get(key);
  if (!edge) {
    edge = {
      time: 0,
      timePercentage: 0,
      caller: network.nodes[callerIndex],
      callee: network.nodes[calleeIndex],
    };
    network.edges.set(key, edge);
    network.nodes[callerIndex].edges.push(edge);
  }
  return edge;
}

/** Convert raw pprof JSON into a PProfNetwork. */
export function pprofDataToNetwork(data: PProfData): PProfNetwork {
  const network = createNetwork();

  for (const func of data.Function) {
    network.nodes.push(createNode(func));
  }

  let totalTime = 0;
  for (const sample of data.Sample) {
    totalTime += sample.Value[0];

    for (let i = 0; i < sample.Location.length; i++) {
      const loc = sample.Location[i];
      const node = network.nodes[loc.Line[0].Function.ID - 1];
      node.time += sample.Value[0];
      if (i === 0) {
        node.selfTime += sample.Value[0];
      }
    }

    for (let i = 0; i < sample.Location.length - 1; i++) {
      const callerIdx = sample.Location[i + 1].Line[0].Function.ID - 1;
      const calleeIdx = sample.Location[i].Line[0].Function.ID - 1;
      const edge = getOrCreateEdge(network, callerIdx, calleeIdx);
      edge.time += sample.Value[0];
    }
  }

  if (totalTime > 0) {
    for (const node of network.nodes) {
      node.selfTimePercentage = node.selfTime / totalTime;
      node.timePercentage = node.time / totalTime;
    }
    for (const edge of network.edges.values()) {
      edge.timePercentage = edge.time / totalTime;
    }
  }

  network.nodes.sort((a, b) => b.time - a.time);
  for (let i = 0; i < network.nodes.length; i++) {
    network.nodes[i].index = i;
  }

  return network;
}
