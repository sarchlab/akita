import { useState } from "react";
import { useComponentNames } from "../hooks/useComponentNames";
import { useSegments } from "../hooks/useSegments";
import ComponentList from "../components/ComponentList";
import SegmentSelector from "../components/SegmentSelector";
import type { Segment } from "../types/task";
import { smartString } from "../utils/smartValue";

/**
 * Dashboard page: shows simulation overview with component list,
 * segment selector, and summary statistics.
 */
export default function DashboardPage() {
  const { names, loading: namesLoading, error: namesError } = useComponentNames();
  const { data: segData, loading: segLoading } = useSegments();
  const [selectedSegIdx, setSelectedSegIdx] = useState(-1);

  const segments = segData?.segments ?? [];
  const segmentsEnabled = segData?.enabled ?? false;

  // Compute overall time range from segments
  let timeRangeStart: number | null = null;
  let timeRangeEnd: number | null = null;
  if (segments.length > 0) {
    timeRangeStart = Math.min(...segments.map((s) => s.start_time));
    timeRangeEnd = Math.max(...segments.map((s) => s.end_time));
  }

  const handleSegmentSelect = (idx: number, _seg: Segment | null) => {
    setSelectedSegIdx(idx);
  };

  return (
    <div className="container-fluid">
      <div className="row">
        {/* ── Left: Component list ──────────────────────── */}
        <div className="col-lg-8 col-md-7">
          <h4 className="mb-3">Dashboard</h4>

          {/* Summary stats row */}
          <div className="row g-3 mb-3">
            <div className="col-auto">
              <div className="card">
                <div className="card-body py-2 px-3">
                  <div className="text-muted small">Components</div>
                  <div className="fw-bold fs-5">
                    {namesLoading ? "…" : names.length}
                  </div>
                </div>
              </div>
            </div>

            <div className="col-auto">
              <div className="card">
                <div className="card-body py-2 px-3">
                  <div className="text-muted small">Segments</div>
                  <div className="fw-bold fs-5">
                    {segLoading ? "…" : segments.length}
                  </div>
                </div>
              </div>
            </div>

            {timeRangeStart != null && timeRangeEnd != null && (
              <div className="col-auto">
                <div className="card">
                  <div className="card-body py-2 px-3">
                    <div className="text-muted small">Time Range</div>
                    <div className="fw-bold" style={{ fontSize: 14 }}>
                      {smartString(timeRangeStart)} →{" "}
                      {smartString(timeRangeEnd)}
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* Component list */}
          <ComponentList
            names={names}
            loading={namesLoading}
            error={namesError}
          />
        </div>

        {/* ── Right: Segment selector & info ─────────── */}
        <div className="col-lg-4 col-md-5">
          <h5 className="mb-3">Segments</h5>

          {segmentsEnabled ? (
            <div>
              <SegmentSelector
                selected={selectedSegIdx}
                onSelect={handleSegmentSelect}
              />

              {/* Show selected segment info */}
              {selectedSegIdx >= 0 && segments[selectedSegIdx] && (
                <div className="card mt-3">
                  <div className="card-body">
                    <h6 className="card-title">
                      Segment {selectedSegIdx + 1}
                    </h6>
                    <dl className="row mb-0" style={{ fontSize: 13 }}>
                      <dt className="col-4">Start</dt>
                      <dd className="col-8">
                        {smartString(segments[selectedSegIdx].start_time)}
                      </dd>
                      <dt className="col-4">End</dt>
                      <dd className="col-8">
                        {smartString(segments[selectedSegIdx].end_time)}
                      </dd>
                      <dt className="col-4">Duration</dt>
                      <dd className="col-8">
                        {smartString(
                          segments[selectedSegIdx].end_time -
                            segments[selectedSegIdx].start_time,
                        )}
                      </dd>
                    </dl>
                  </div>
                </div>
              )}

              {/* Segment overview list */}
              {segments.length > 0 && (
                <div className="mt-3">
                  <h6>All Segments</h6>
                  <div
                    className="list-group list-group-flush"
                    style={{ maxHeight: 300, overflowY: "auto", fontSize: 12 }}
                  >
                    {segments.map((seg, i) => (
                      <button
                        key={i}
                        type="button"
                        className={`list-group-item list-group-item-action py-1 ${
                          selectedSegIdx === i ? "active" : ""
                        }`}
                        onClick={() =>
                          handleSegmentSelect(i, seg)
                        }
                      >
                        <strong>#{i + 1}</strong>{" "}
                        {smartString(seg.start_time)} →{" "}
                        {smartString(seg.end_time)}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <p className="text-muted">
              {segLoading
                ? "Loading segment data…"
                : "Segment tracing is not enabled for this simulation."}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
