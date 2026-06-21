import WidgetCard from "./WidgetCard";
import DashboardWidget from "./DashboardWidget";
import { useBlocked } from "../hooks/useBlocked";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useSegments } from "../hooks/useSegments";
import { useElementSize } from "../hooks/useElementSize";
import { DASHBOARD_DEFAULTS } from "../utils/viewState.mjs";

const GAP = 8;

interface BlockedComponentsWidgetProps {
  expandHref?: string;
}

// BlockedComponentsWidget shows the two components whose tasks spent the most
// time blocked, each as a full dashboard chart — so enlarging the widget is, in
// effect, the dashboard focused on the worst offenders.
export default function BlockedComponentsWidget({
  expandHref,
}: BlockedComponentsWidgetProps) {
  const { data, loading, error } = useBlocked();
  const { startTime, endTime } = useSimulationRange();
  const { data: segments } = useSegments();
  const { ref, size } = useElementSize<HTMLDivElement>();

  const top = (data ?? []).filter((c) => c.blocked_time > 0).slice(0, 2);
  const widgetWidth = Math.max(160, size.width);
  const widgetHeight = Math.max(120, (size.height - GAP) / 2);

  return (
    <WidgetCard
      title="Most blocked components"
      expandHref={expandHref}
      contentClassName="p-2"
    >
      {loading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : error ? (
        <div className="text-sm text-destructive">{error}</div>
      ) : top.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No blocking recorded in this trace.
        </div>
      ) : (
        <div
          ref={ref}
          className="flex h-full min-h-0 flex-col"
          style={{ gap: GAP }}
        >
          {top.map((c) => (
            <DashboardWidget
              key={c.component}
              name={c.component}
              width={widgetWidth}
              height={widgetHeight}
              startTime={startTime}
              endTime={endTime}
              dataStartTime={startTime}
              dataEndTime={endTime}
              dataPending={false}
              primaryAxis={DASHBOARD_DEFAULTS.primary}
              secondaryAxis={DASHBOARD_DEFAULTS.secondary}
              segments={segments?.segments ?? []}
              segmentsEnabled={segments?.enabled ?? false}
              onTimeRangeChange={() => {}}
            />
          ))}
        </div>
      )}
    </WidgetCard>
  );
}
