import { useEffect, useState } from "react";

/** A single data point in the time series. */
export interface TimeValue {
  time: number;
  value: number;
}

/** Shape returned by /api/compinfo. */
export interface ComponentInfo {
  name: string;
  info_type: string;
  start_time: number;
  end_time: number;
  data: TimeValue[];
}

interface CompInfoState {
  info: ComponentInfo | null;
  loading: boolean;
  error: string | null;
}

/**
 * Fetch component time-series data from /api/compinfo.
 *
 * @param compName  The component name (`where` param).
 * @param infoType  One of ReqInCount, ReqCompleteCount, AvgLatency, ConcurrentTask, BufferPressure, PendingReqOut.
 * @param startTime Start time of the range.
 * @param endTime   End time of the range.
 * @param numDots   Number of data points (default 40).
 */
export function useCompInfo(
  compName: string,
  infoType: string,
  startTime: number,
  endTime: number,
  numDots = 40,
): CompInfoState {
  const [info, setInfo] = useState<ComponentInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!compName || !infoType || startTime >= endTime) {
      setInfo(null);
      return;
    }

    const controller = new AbortController();
    setLoading(true);
    setError(null);

    const params = new URLSearchParams();
    params.set("where", compName);
    params.set("info_type", infoType);
    params.set("start_time", String(startTime));
    params.set("end_time", String(endTime));
    params.set("num_dots", String(numDots));

    fetch(`/api/compinfo?${params.toString()}`, { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json: ComponentInfo) => {
        setInfo(json);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, [compName, infoType, startTime, endTime, numDots]);

  return { info, loading, error };
}
