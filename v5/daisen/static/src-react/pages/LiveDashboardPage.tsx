import { useMode } from "../hooks/useMode";
import ProgressBars from "../components/live/ProgressBars";
import HangAnalyzer from "../components/live/HangAnalyzer";

/**
 * Live Dashboard — shows progress bars and hang analyzer.
 * Only accessible in "live" mode.
 */
export default function LiveDashboardPage() {
  const { mode, loading } = useMode();

  if (loading) {
    return <p className="text-muted">Loading…</p>;
  }

  if (mode !== "live") {
    return (
      <div className="container-fluid">
        <p className="text-muted">
          Live dashboard is only available in live mode.
        </p>
      </div>
    );
  }

  return (
    <div className="container-fluid">
      <h4 className="mb-3">Live Dashboard</h4>

      <div className="row">
        {/* Progress bars section */}
        <div className="col-lg-6 col-md-12 mb-3">
          <div className="card">
            <div className="card-body">
              <ProgressBars />
            </div>
          </div>
        </div>

        {/* Hang analyzer section */}
        <div className="col-lg-6 col-md-12 mb-3">
          <div className="card">
            <div className="card-body">
              <HangAnalyzer />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
