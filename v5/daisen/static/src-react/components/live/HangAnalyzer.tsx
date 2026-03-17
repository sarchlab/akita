import { useCallback, useEffect, useRef, useState } from "react";
import { useMode } from "../../hooks/useMode";

/** Shape of a single buffer entry from /api/hangdetector/buffers */
interface BufferEntry {
  buffer: string;
  level: number;
  cap: number;
}

type SortField = "level" | "percent";

/**
 * HangAnalyzer — fetches /api/hangdetector/buffers and displays a
 * sortable, auto-refreshing table of buffer levels.
 * Only renders when mode === "live".
 */
export default function HangAnalyzer() {
  const { mode } = useMode();
  const [buffers, setBuffers] = useState<BufferEntry[]>([]);
  const [sort, setSort] = useState<SortField>("level");
  const [running, setRunning] = useState(true);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const controllerRef = useRef<AbortController | null>(null);

  const fetchBuffers = useCallback(
    (sortField: SortField) => {
      controllerRef.current?.abort();
      const controller = new AbortController();
      controllerRef.current = controller;

      fetch(`/api/hangdetector/buffers?sort=${sortField}&limit=40`, {
        signal: controller.signal,
      })
        .then((res) => res.json())
        .then((data: BufferEntry[] | null) => {
          setBuffers(data ?? []);
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
        });
    },
    [],
  );

  // Start / stop auto-refresh
  useEffect(() => {
    if (mode !== "live") return;

    if (running) {
      fetchBuffers(sort);
      timerRef.current = setInterval(() => fetchBuffers(sort), 2000);
    }

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      controllerRef.current?.abort();
    };
  }, [mode, running, sort, fetchBuffers]);

  const handleSortChange = (field: SortField) => {
    setSort(field);
  };

  const toggleRunning = () => {
    setRunning((prev) => !prev);
  };

  if (mode !== "live") return null;

  return (
    <div>
      <h6 className="mb-2">Hang Analyzer</h6>

      {/* Toolbar */}
      <div className="btn-toolbar mb-2 gap-2">
        <div className="input-group input-group-sm" role="group">
          <span className="input-group-text">Sort by:</span>
          <button
            type="button"
            className={`btn ${sort === "level" ? "btn-primary" : "btn-outline-primary"} btn-sm`}
            onClick={() => handleSortChange("level")}
          >
            Size
          </button>
          <button
            type="button"
            className={`btn ${sort === "percent" ? "btn-primary" : "btn-outline-primary"} btn-sm`}
            onClick={() => handleSortChange("percent")}
          >
            Percent
          </button>
        </div>

        <button
          type="button"
          className={`btn btn-sm ${running ? "btn-primary" : "btn-outline-primary"}`}
          onClick={toggleRunning}
        >
          {running ? "Stop Refresh" : "Auto Refresh"}
        </button>
      </div>

      {/* Buffer table */}
      <div style={{ maxHeight: 500, overflowY: "auto" }}>
        <table className="table table-sm table-striped mb-0">
          <thead>
            <tr>
              <th>Buffer</th>
              <th style={{ width: 80, textAlign: "right" }}>Size</th>
              <th style={{ width: 80, textAlign: "right" }}>Cap</th>
            </tr>
          </thead>
          <tbody>
            {buffers.length === 0 ? (
              <tr>
                <td colSpan={3} className="text-muted text-center">
                  No buffer data
                </td>
              </tr>
            ) : (
              buffers.map((b, i) => (
                <tr key={`${b.buffer}-${i}`}>
                  <td className="text-break">{b.buffer}</td>
                  <td style={{ textAlign: "right" }}>{b.level}</td>
                  <td style={{ textAlign: "right" }}>{b.cap}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
