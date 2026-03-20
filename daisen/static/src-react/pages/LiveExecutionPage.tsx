import { useMode } from "../hooks/useMode";
import ResourceMonitor from "../components/live/ResourceMonitor";

/**
 * LiveExecutionPage — resource monitor and profiling for live mode.
 */
export default function LiveExecutionPage() {
  const { mode, loading } = useMode();

  if (loading) {
    return <p className="text-muted">Loading…</p>;
  }

  if (mode !== "live") {
    return (
      <div className="container-fluid">
        <p className="text-muted">
          Execution page is only available in live mode.
        </p>
      </div>
    );
  }

  return (
    <div className="container-fluid">
      <h4 className="mb-3">Execution</h4>

      <div className="row">
        <div className="col-12 mb-3">
          <div className="card">
            <div className="card-body">
              <ResourceMonitor />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
