import { useCallback, useEffect, useState } from "react";

/** Internal tree node built from dot-separated component names. */
interface TreeNode {
  name: string;
  fullName: string;
  children: Map<string, TreeNode>;
}

/** Props for the top-level ComponentTree panel. */
interface ComponentTreeProps {
  onSelectComponent: (fullName: string) => void;
}

/** Indentation per tree depth level (px). */
const INDENT_PX = 25;

/* ------------------------------------------------------------------ */
/*  Tree construction                                                  */
/* ------------------------------------------------------------------ */

function buildTree(names: string[]): TreeNode {
  const root: TreeNode = { name: "", fullName: "", children: new Map() };

  for (const compName of names) {
    const tokens = compName.split(".");
    let node = root;
    let path = "";

    for (const token of tokens) {
      path = path ? `${path}.${token}` : token;

      if (!node.children.has(token)) {
        node.children.set(token, {
          name: token,
          fullName: path,
          children: new Map(),
        });
      }

      node = node.children.get(token)!;
    }
  }

  return root;
}

/* ------------------------------------------------------------------ */
/*  Single tree-node component                                         */
/* ------------------------------------------------------------------ */

function TreeNodeView({
  node,
  depth,
  onSelect,
}: {
  node: TreeNode;
  depth: number;
  onSelect: (fullName: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const isLeaf = node.children.size === 0;

  const toggle = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      if (isLeaf) {
        onSelect(node.fullName);
      } else {
        setExpanded((prev) => !prev);
      }
    },
    [isLeaf, node.fullName, onSelect],
  );

  const indent = depth * INDENT_PX;

  if (isLeaf) {
    return (
      <div
        className="tree-leaf"
        style={{
          paddingLeft: indent,
          paddingTop: 2,
          paddingBottom: 2,
          paddingRight: 4,
          cursor: "pointer",
          userSelect: "none",
        }}
        onClick={toggle}
        role="button"
        tabIndex={0}
      >
        <span className="text-muted me-1">–</span>
        <span>{node.name}</span>
      </div>
    );
  }

  const sortedChildren = Array.from(node.children.values()).sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  return (
    <div>
      <div
        style={{
          paddingLeft: indent,
          paddingTop: 2,
          paddingBottom: 2,
          paddingRight: 4,
          cursor: "pointer",
          userSelect: "none",
        }}
        onClick={toggle}
        role="button"
        tabIndex={0}
      >
        <i
          className={`fa-solid fa-xs me-1 ${
            expanded ? "fa-chevron-down" : "fa-chevron-right"
          }`}
        />
        <span className="fw-semibold">{node.name}</span>
      </div>

      {expanded && (
        <div>
          {sortedChildren.map((child) => (
            <TreeNodeView
              key={child.fullName}
              node={child}
              depth={depth + 1}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main ComponentTree                                                 */
/* ------------------------------------------------------------------ */

/**
 * Fetches component names from /api/list_components, builds a
 * hierarchical tree, and renders expandable/collapsible nodes.
 * Leaf nodes (actual components) are clickable and fire onSelectComponent.
 *
 * Only meaningful in live mode — the parent should guard rendering.
 */
export default function ComponentTree({ onSelectComponent }: ComponentTreeProps) {
  const [names, setNames] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    fetch("/api/list_components", { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json: string[]) => {
        setNames(json ?? []);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, []);

  const tree = buildTree(names);

  const sortedRoots = Array.from(tree.children.values()).sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  if (loading) {
    return (
      <div className="p-2 text-muted">
        <div className="spinner-border spinner-border-sm me-2" />
        Loading components…
      </div>
    );
  }

  if (error) {
    return <div className="alert alert-danger py-1 m-2">{error}</div>;
  }

  if (names.length === 0) {
    return <div className="p-2 text-muted">No components found.</div>;
  }

  return (
    <div className="component-tree" style={{ fontSize: "0.9rem" }}>
      {sortedRoots.map((child) => (
        <TreeNodeView
          key={child.fullName}
          node={child}
          depth={0}
          onSelect={onSelectComponent}
        />
      ))}
    </div>
  );
}
