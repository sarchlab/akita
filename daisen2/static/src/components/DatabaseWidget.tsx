import WidgetCard from "./WidgetCard";
import { useDBInfo } from "../hooks/useDBInfo";
import { useDBActivity } from "../hooks/useDBActivity";
import type { DBActivity, DBTableInfo } from "../types/db";

// formatBytes renders a byte count with a binary unit, dropping the decimal for
// large/whole values so a column of sizes stays easy to scan.
function formatBytes(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i += 1;
  }
  const decimals = i === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(decimals)} ${units[i]}`;
}

function formatInt(n: number): string {
  return n.toLocaleString();
}

function formatElapsed(seconds: number): string {
  if (seconds < 60) return `${seconds.toFixed(0)}s`;
  const m = Math.floor(seconds / 60);
  return `${m}m ${Math.round(seconds % 60)}s`;
}

// ActivityPanel shows what the database is doing right now — index builds, heavy
// queries, the size scan — each with its elapsed time. On a large trace these
// operations take minutes, so this turns the old bare "Loading…" into a live,
// legible status. It renders nothing when the database is idle.
function ActivityPanel({ activity }: { activity: DBActivity[] }) {
  if (activity.length === 0) return null;

  return (
    <div className="mb-4 rounded-md border border-amber-300 bg-amber-50 px-3 py-2">
      <div className="mb-1 text-xs font-medium uppercase tracking-wide text-amber-700">
        Working on the database
      </div>
      <ul className="flex flex-col gap-1.5">
        {activity.map((a) => (
          <li key={a.id} className="flex items-center gap-2 text-sm">
            <span className="daisen-widget-update-spinner shrink-0" aria-hidden="true" />
            <span className="font-medium">{a.name}</span>
            {a.detail ? (
              <span className="truncate font-mono text-xs text-muted-foreground">
                {a.detail}
              </span>
            ) : null}
            <span className="ml-auto shrink-0 tabular-nums text-xs text-amber-700">
              {formatElapsed(a.elapsed_seconds)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// SummaryStat is one labelled figure in the header strip.
function SummaryStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span className="text-sm tabular-nums">{value}</span>
    </div>
  );
}

// TableRow renders one table as a labelled stacked bar — data vs. index bytes —
// scaled against the largest table's footprint, so the rows that dominate the
// file (and how much of that is indexes) are obvious at a glance.
function TableRow({
  table,
  maxBytes,
  hasSizes,
}: {
  table: DBTableInfo;
  maxBytes: number;
  hasSizes: boolean;
}) {
  const total = table.data_bytes + table.index_bytes;
  const scale = maxBytes > 0 ? total / maxBytes : 0;
  const dataFrac = total > 0 ? table.data_bytes / total : 0;

  return (
    <div className="flex flex-col gap-1 py-1.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="truncate font-mono text-sm">{table.name}</span>
        <span className="shrink-0 tabular-nums text-xs text-muted-foreground">
          {formatInt(table.rows)} rows
          {hasSizes ? <> · {formatBytes(total)}</> : null}
        </span>
      </div>
      {hasSizes ? (
        <div className="flex items-center gap-2">
          <div className="h-2 flex-1 overflow-hidden rounded-sm bg-muted">
            <div className="flex h-full" style={{ width: `${scale * 100}%` }}>
              <div
                className="h-full bg-sky-500"
                style={{ width: `${dataFrac * 100}%` }}
                title={`data ${formatBytes(table.data_bytes)}`}
              />
              <div
                className="h-full bg-violet-400"
                style={{ width: `${(1 - dataFrac) * 100}%` }}
                title={`indexes ${formatBytes(table.index_bytes)}`}
              />
            </div>
          </div>
          <span className="w-28 shrink-0 text-right tabular-nums text-xs text-muted-foreground">
            {formatBytes(table.data_bytes)} + {formatBytes(table.index_bytes)} idx
          </span>
        </div>
      ) : null}
    </div>
  );
}

interface DatabaseWidgetProps {
  expandHref?: string;
  bare?: boolean;
}

// DatabaseWidget surfaces the trace's underlying SQLite database: its tables
// with row counts and on-disk data/index sizes, plus a live view of the
// operations the database is running. It answers "why is this trace so big?"
// (usually: indexes) and "what is it doing?" during the long index builds and
// queries a large trace triggers.
export default function DatabaseWidget({ expandHref, bare }: DatabaseWidgetProps) {
  const { data, computing, loading, error } = useDBInfo();
  const activity = useDBActivity();

  const indexShare =
    data && data.has_sizes && data.data_bytes + data.index_bytes > 0
      ? data.index_bytes / (data.data_bytes + data.index_bytes)
      : null;
  const maxBytes = data
    ? Math.max(0, ...data.tables.map((t) => t.data_bytes + t.index_bytes))
    : 0;

  return (
    <WidgetCard
      title="Database"
      expandHref={expandHref}
      bare={bare}
      headerRight={
        data ? (
          <span className="tabular-nums text-xs text-muted-foreground">
            {formatBytes(data.file_bytes)}
          </span>
        ) : null
      }
    >
      <ActivityPanel activity={activity} />

      {loading && !data ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : error && !data ? (
        <div className="text-sm text-destructive">{error}</div>
      ) : computing && !data ? (
        // Cold cache: /api/db_info returns {computing:true, info:null} while the
        // background dbstat/COUNT scan runs. Show that it is measuring rather than
        // "No tables found.", which on a large trace would otherwise persist for
        // the whole scan.
        <div className="text-sm text-muted-foreground">Measuring database…</div>
      ) : !data || data.tables.length === 0 ? (
        <div className="text-sm text-muted-foreground">No tables found.</div>
      ) : (
        <div className="flex min-h-0 flex-col gap-3">
          <dl className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4">
            <SummaryStat label="File size" value={formatBytes(data.file_bytes)} />
            <SummaryStat label="Total rows" value={formatInt(data.total_rows)} />
            {data.has_sizes ? (
              <SummaryStat label="Data" value={formatBytes(data.data_bytes)} />
            ) : null}
            {data.has_sizes ? (
              <SummaryStat
                label="Indexes"
                value={`${formatBytes(data.index_bytes)}${
                  indexShare !== null ? ` (${Math.round(indexShare * 100)}%)` : ""
                }`}
              />
            ) : null}
          </dl>

          {!data.has_sizes ? (
            <div className="rounded bg-muted px-2 py-1 text-xs text-muted-foreground">
              On-disk sizes unavailable — this SQLite build lacks the dbstat
              module. Showing row counts only.
            </div>
          ) : null}

          {computing ? (
            <div className="text-xs text-muted-foreground">Measuring sizes…</div>
          ) : null}

          <div className="min-h-0 divide-y divide-border overflow-auto">
            {data.tables.map((t) => (
              <TableRow
                key={t.name}
                table={t}
                maxBytes={maxBytes}
                hasSizes={data.has_sizes}
              />
            ))}
          </div>
        </div>
      )}
    </WidgetCard>
  );
}
