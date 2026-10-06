import { ChevronDown, ChevronRight } from "lucide-react";
import type { LocationNode } from "../utils/locationTree";
import { cn } from "../lib/utils";

// LocationSubtree renders the tree of locations beneath a scope, with every row
// clickable to jump into that location. Branches are collapsible (chevron),
// collapsed by default, so a deep scope stays compact instead of dumping its
// whole subtree at once.
export default function LocationSubtree({
  nodes,
  onNavigate,
  expanded,
  onToggle,
}: {
  nodes: LocationNode[];
  onNavigate: (path: string) => void;
  expanded: Set<string>;
  onToggle: (path: string) => void;
}) {
  return (
    <ul className="space-y-0.5">
      {nodes.map((node) => {
        const isBranch = node.children.length > 0;
        const open = isBranch && expanded.has(node.path);
        return (
          <li key={node.path}>
            <div className="flex items-center">
              {isBranch ? (
                <button
                  type="button"
                  className="flex h-6 w-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-primary"
                  onClick={() => onToggle(node.path)}
                  aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
                >
                  {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                </button>
              ) : (
                // A hollow dot marks a leaf (no children to expand).
                <span className="flex h-6 w-5 shrink-0 items-center justify-center" aria-hidden="true">
                  <span className="h-1.5 w-1.5 rounded-full border border-muted-foreground/50" />
                </span>
              )}
              <button
                type="button"
                className={cn(
                  "min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-xs transition-colors hover:bg-muted hover:text-primary",
                  isBranch ? "font-medium text-foreground" : "text-muted-foreground",
                )}
                onClick={() => onNavigate(node.path)}
                title={node.path}
              >
                {node.name}
              </button>
            </div>
            {/* Indent guide: a left border ties each child row back to its parent. */}
            {isBranch && open && (
              <div className="ml-2 border-l border-border pl-1.5">
                <LocationSubtree nodes={node.children} onNavigate={onNavigate} expanded={expanded} onToggle={onToggle} />
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
