import { useCallback } from "react";
import { useMode } from "../../hooks/useMode";
import { useEngineTime } from "../../hooks/useEngineTime";

/**
 * Engine control buttons (Pause / Continue / Run) plus the current
 * simulation time.  Only renders when mode === "live".
 */
export default function EngineControlPanel() {
  const { mode } = useMode();
  const { now, loading: timeLoading } = useEngineTime(1000);

  const pause = useCallback(() => {
    fetch("/api/pause").catch(console.error);
  }, []);

  const cont = useCallback(() => {
    fetch("/api/continue").catch(console.error);
  }, []);

  const run = useCallback(() => {
    fetch("/api/run").catch(console.error);
  }, []);

  if (mode !== "live") return null;

  return (
    <div className="d-flex align-items-center gap-2">
      <span className="navbar-text me-2" id="now-label">
        {timeLoading ? "…" : `Time: ${now ?? "N/A"}`}
      </span>

      <div className="btn-group btn-group-sm" role="group">
        <button
          type="button"
          className="btn btn-outline-warning"
          onClick={pause}
          title="Pause"
        >
          <i className="fas fa-pause" />
        </button>

        <button
          type="button"
          className="btn btn-outline-success"
          onClick={cont}
          title="Continue"
        >
          <i className="fas fa-play" />
        </button>

        <button
          type="button"
          className="btn btn-outline-info"
          onClick={run}
          title="Run"
        >
          <i className="fas fa-forward" />
        </button>
      </div>
    </div>
  );
}
