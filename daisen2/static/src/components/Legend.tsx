import { BlockingReasonsHelp, TaskTypesHelp } from "./HelpTopics";
import { cn } from "../lib/utils";
import { wavyPath } from "../utils/milestoneViz";
import type { ColorMode } from "../utils/taskColorCoder";

export function SectionLabel({ children }: { children: string }) {
  return (
    <div className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{children}</div>
  );
}

// A Kind / Kind-What segmented toggle, shared by the Tasks and Blocking-reasons
// sections so each can switch its own coloring granularity independently.
function ColorModeToggle({
  mode,
  onChange,
  label,
}: {
  mode: ColorMode;
  onChange: (mode: ColorMode) => void;
  label: string;
}) {
  return (
    <div className="inline-flex shrink-0 overflow-hidden rounded border text-[10px] font-medium" role="group" aria-label={label}>
      {(["kind", "kind-what"] as const).map((m) => (
        <button
          key={m}
          type="button"
          onClick={() => onChange(m)}
          aria-pressed={mode === m}
          className={cn(
            "px-1.5 py-0.5 transition-colors",
            m === "kind-what" && "border-l",
            mode === m ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted",
          )}
        >
          {m === "kind" ? "Kind" : "Kind-What"}
        </button>
      ))}
    </div>
  );
}

interface LegendProps {
  taskKeys: string[];
  colorMap: Record<string, string>;
  blockingReasons: string[];
  // The blocking-reason colors (a separate warm family from the task colors). Falls
  // back to the task colorMap if not given.
  milestoneColorMap?: Record<string, string>;
  // Task color-mode toggle (Kind / Kind-What). Rendered only when onColorMode is
  // given — the component view drives its task-count bands with it; the task view
  // has no such control.
  colorMode?: ColorMode;
  onColorMode?: (mode: ColorMode) => void;
  // The independent blocking-reason color-mode toggle, shown in the Blocking
  // reasons section when onMilestoneColorMode is given.
  milestoneColorMode?: ColorMode;
  onMilestoneColorMode?: (mode: ColorMode) => void;
  // Hover-highlight wiring. When omitted the swatches are a static key, with no
  // dimming and no chart highlight (the task view).
  highlightedKey?: string | null;
  onHighlight?: (key: string | null) => void;
  highlightedReason?: string | null;
  onHighlightReason?: (kind: string | null) => void;
}

// Legend is the shared task-type + blocking-reason key used by both the component
// view and the task view, so swatches, colors and glyphs read identically. The
// component view passes the color-mode toggle and hover-highlight handlers; the
// task view passes neither and gets a static key.
export default function Legend({
  taskKeys,
  colorMap,
  blockingReasons,
  milestoneColorMap,
  colorMode,
  onColorMode,
  milestoneColorMode,
  onMilestoneColorMode,
  highlightedKey = null,
  onHighlight,
  highlightedReason = null,
  onHighlightReason,
}: LegendProps) {
  const reasonColorMap = milestoneColorMap ?? colorMap;
  if (taskKeys.length === 0 && blockingReasons.length === 0) return null;

  return (
    <section>
      {taskKeys.length > 0 && (
        <>
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-1">
              <SectionLabel>Tasks</SectionLabel>
              <TaskTypesHelp />
            </div>
            {onColorMode ? (
              <ColorModeToggle label="Color tasks by" mode={colorMode ?? "kind-what"} onChange={onColorMode} />
            ) : null}
          </div>
          <ul className="mb-3 mt-2 space-y-0.5">
            {taskKeys.map((key) => {
              const active = highlightedKey === key;
              const dimmed = highlightedKey !== null && !active;
              return (
                <li key={key}>
                  <button
                    type="button"
                    className={cn(
                      "flex w-full items-center gap-2 rounded px-1.5 py-1 text-left text-xs transition-colors hover:bg-muted",
                      active && "bg-primary/10",
                      dimmed && "opacity-40",
                    )}
                    onMouseEnter={() => onHighlight?.(key)}
                    onMouseLeave={() => onHighlight?.(null)}
                    onFocus={() => onHighlight?.(key)}
                    onBlur={() => onHighlight?.(null)}
                  >
                    <span
                      className="h-3 w-5 shrink-0 rounded-sm border border-black/30"
                      style={{ backgroundColor: colorMap[key] ?? "#9ca3af" }}
                    />
                    <span className="truncate">{key}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}

      {blockingReasons.length > 0 && (
        <>
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-1">
              <SectionLabel>Blocking reasons</SectionLabel>
              <BlockingReasonsHelp />
            </div>
            {onMilestoneColorMode ? (
              <ColorModeToggle
                label="Color blocking reasons by"
                mode={milestoneColorMode ?? "kind-what"}
                onChange={onMilestoneColorMode}
              />
            ) : null}
          </div>
          <ul className="mt-2 space-y-0.5">
            {blockingReasons.map((kind) => {
              const color = reasonColorMap[kind] ?? "#9ca3af";
              const active = highlightedReason === kind;
              const dimmed = highlightedReason !== null && !active;
              return (
                <li key={kind}>
                  <button
                    type="button"
                    className={cn(
                      "flex w-full items-center gap-2 rounded px-1.5 py-1 text-left text-xs transition-colors hover:bg-muted",
                      active && "bg-primary/10",
                      dimmed && "opacity-40",
                    )}
                    onMouseEnter={() => onHighlightReason?.(kind)}
                    onMouseLeave={() => onHighlightReason?.(null)}
                    onFocus={() => onHighlightReason?.(kind)}
                    onBlur={() => onHighlightReason?.(null)}
                  >
                    {/* Two glyphs: the wavy line (task/gantt view) and a borderless
                        block (stacked area chart), both colored by the reason. */}
                    <span className="flex shrink-0 items-center gap-1">
                      <svg width="22" height="12" aria-hidden="true">
                        <path
                          d={wavyPath(1, 21, 6, 3, 3)}
                          fill="none"
                          stroke={color}
                          strokeWidth={1.5}
                          strokeLinecap="round"
                        />
                      </svg>
                      <span className="inline-block h-3 w-4 rounded-sm" style={{ backgroundColor: color }} />
                    </span>
                    <span className="truncate">{kind}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}
