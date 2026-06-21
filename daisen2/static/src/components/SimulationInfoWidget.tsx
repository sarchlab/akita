import { useMemo, type ReactNode } from "react";
import WidgetCard from "./WidgetCard";
import { useSimInfo } from "../hooks/useSimInfo";
import { useTopology } from "../hooks/useTopology";
import { formatVirtualTime } from "../lib/time";
import type { SimInfoEntry } from "../types/overview";

// formatWallClockDuration parses two recorded wall-clock timestamps (e.g.
// "2026-06-20 17:43:37.379284000") and returns their gap, best-effort.
function formatWallClockDuration(start: string, end: string): string | null {
  const toMs = (s: string) => Date.parse(s.replace(" ", "T"));
  const a = toMs(start);
  const b = toMs(end);
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return null;
  const seconds = (b - a) / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)} s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${Math.round(seconds % 60)}s`;
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

interface SimulationInfoWidgetProps {
  expandHref?: string;
  bare?: boolean;
}

export default function SimulationInfoWidget({
  expandHref,
  bare,
}: SimulationInfoWidgetProps) {
  const { data, loading, error } = useSimInfo();
  const { data: topology } = useTopology();
  const componentCount = topology?.components.length;

  const lookup = useMemo(() => {
    const map = new Map<string, string>();
    (data ?? []).forEach((e: SimInfoEntry) => map.set(e.property, e.value));
    return map;
  }, [data]);

  const startVT = parseFloat(lookup.get("Start Virtual Time") ?? "");
  const endVT = parseFloat(lookup.get("End Virtual Time") ?? "");
  const hasVT = Number.isFinite(startVT) && Number.isFinite(endVT);
  const command = lookup.get("Command");
  const startWall = lookup.get("Start Time");
  const endWall = lookup.get("End Time");
  const wallDuration =
    startWall && endWall ? formatWallClockDuration(startWall, endWall) : null;

  return (
    <WidgetCard title="Simulation" expandHref={expandHref} bare={bare}>
      {loading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : error ? (
        <div className="text-sm text-destructive">{error}</div>
      ) : !data || data.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No simulation metadata recorded.
        </div>
      ) : (
        <dl className="grid grid-cols-2 gap-x-6 gap-y-4">
          {command ? (
            <div className="col-span-2 flex flex-col gap-0.5">
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Command
              </dt>
              <dd className="break-all rounded bg-muted px-2 py-1 font-mono text-xs">
                {command}
              </dd>
            </div>
          ) : null}

          {typeof componentCount === "number" ? (
            <Row label="Components">{componentCount}</Row>
          ) : null}

          {hasVT ? (
            <Row label="Virtual time">{formatVirtualTime(endVT - startVT)}</Row>
          ) : null}

          {hasVT ? (
            <Row label="Virtual span">
              {formatVirtualTime(startVT)} → {formatVirtualTime(endVT)}
            </Row>
          ) : null}

          {wallDuration ? <Row label="Wall-clock">{wallDuration}</Row> : null}

          {startWall ? <Row label="Started">{startWall}</Row> : null}

          {lookup.get("Working Directory") ? (
            <div className="col-span-2">
              <Row label="Working directory">
                <span className="break-all font-mono text-xs">
                  {lookup.get("Working Directory")}
                </span>
              </Row>
            </div>
          ) : null}
        </dl>
      )}
    </WidgetCard>
  );
}
