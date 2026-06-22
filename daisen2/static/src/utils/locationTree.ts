// locationTree turns the flat list of dotted location names (e.g. "TLB.req_in",
// "TLB.Top.incoming", "L2Cache.bank") into a prefix tree. The dot is the
// hierarchy separator, so a component like "TLB" becomes an internal node whose
// descendants are its task-facet rows. This drives the component drill-down:
// clicking an internal node shows every leaf beneath it, and a node's direct
// children populate the next-level dropdown.

export interface LocationNode {
  // Last path segment, used as the display label (e.g. "req_in", "Top", "TLB").
  name: string;
  // Full dotted path from the root (e.g. "TLB.Top"). Empty string for the root.
  path: string;
  // True when this exact path is a real location in the trace (a leaf row a
  // task can sit on). A node may be both a real location and have children if a
  // location name is a prefix of another, but that does not happen today.
  isLocation: boolean;
  children: LocationNode[];
}

function makeNode(name: string, path: string): LocationNode {
  return { name, path, isLocation: false, children: [] };
}

// buildLocationTree builds the prefix tree from the location names. Children are
// kept in natural order so callers render them deterministically.
export function buildLocationTree(names: string[]): LocationNode {
  const root = makeNode("", "");

  for (const fullName of names) {
    if (!fullName) continue;

    const segments = fullName.split(".");
    let node = root;
    let prefix = "";

    for (const segment of segments) {
      prefix = prefix ? `${prefix}.${segment}` : segment;
      let child = node.children.find((c) => c.name === segment);
      if (!child) {
        child = makeNode(segment, prefix);
        node.children.push(child);
      }
      node = child;
    }

    node.isLocation = true;
  }

  sortTree(root);
  return root;
}

function naturalCompare(a: string, b: string) {
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
}

function sortTree(node: LocationNode) {
  node.children.sort((a, b) => naturalCompare(a.name, b.name));
  node.children.forEach(sortTree);
}

// findNode returns the node at the given dotted path, or null if no location
// uses that prefix. The empty path returns the root.
export function findNode(root: LocationNode, path: string): LocationNode | null {
  if (path === "") return root;

  let node: LocationNode = root;
  for (const segment of path.split(".")) {
    const child = node.children.find((c) => c.name === segment);
    if (!child) return null;
    node = child;
  }
  return node;
}

// isInternalNode reports whether the path names a node that has children, i.e. a
// component/port to drill into rather than a single task row.
export function isInternalNode(root: LocationNode, path: string): boolean {
  const node = findNode(root, path);
  return !!node && node.children.length > 0;
}

// isLeafNode reports whether the path names a single task row (a real location
// with no children).
export function isLeafNode(node: LocationNode): boolean {
  return node.children.length === 0;
}

// childPathFor returns the scope's direct child that contains the given full
// location. For scope "TLB" and location "TLB.Top.incoming" it returns
// "TLB.Top"; for "TLB.req_in" it returns "TLB.req_in". Returns the scope itself
// when location equals the scope.
export function childPathFor(scope: string, location: string): string {
  if (location === scope) return scope;
  const prefix = `${scope}.`;
  if (!location.startsWith(prefix)) return location;
  const firstSegment = location.slice(prefix.length).split(".")[0];
  return `${scope}.${firstSegment}`;
}

// breadcrumbSegments splits a dotted path into cumulative (label, path) pairs so
// each ancestor can be rendered as a clickable crumb.
export function breadcrumbSegments(path: string): { label: string; path: string }[] {
  if (!path) return [];
  const segments = path.split(".");
  let prefix = "";
  return segments.map((segment) => {
    prefix = prefix ? `${prefix}.${segment}` : segment;
    return { label: segment, path: prefix };
  });
}
