import { useEffect, useRef, useState } from "react";
import { useRenderReady } from "./useRenderReady";

export interface TimeValue {
  time: number;
  value: number;
}

export interface ComponentInfo {
  name: string;
  info_type: string;
  start_time: number;
  end_time: number;
  data: TimeValue[];
}

export function useCompInfo(
  compName: string,
  infoType: string,
  startTime: number,
  endTime: number,
  numDots = 40,
) {
  const [info, setInfo] = useState<ComponentInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!compName || !infoType || infoType === "-" || startTime >= endTime) {
      setInfo(null);
      return;
    }

    const controller = new AbortController();
    const params = new URLSearchParams({
      where: compName,
      info_type: infoType,
      start_time: String(startTime),
      end_time: String(endTime),
      num_dots: String(numDots),
    });

    setLoading(true);
    setError(null);
    fetch(`/api/compinfo?${params.toString()}`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: ComponentInfo) => setInfo(json))
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        // A superseded (aborted) request must not clear loading for the request
        // that replaced it — that would flip render-ready true while data is stale.
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [compName, infoType, startTime, endTime, numDots]);

  useRenderReady(loading, error !== null);

  return { info, loading, error };
}

// A sample point carrying a per-reason (milestone kind) count.
export interface StackedTimeValue {
  time: number;
  values: Record<string, number>;
}

export interface StackedComponentInfo {
  name: string;
  info_type: string;
  start_time: number;
  end_time: number;
  data: StackedTimeValue[];
  kinds: string[];
}

// useStackedCompInfo fetches a per-reason stacked time series (e.g.
// "ConcurrentTaskMilestones": at each of numDots samples, how many in-flight
// tasks are blocked by each reason). Mirrors useCompInfo but for the stacked
// response shape.
export function useStackedCompInfo(
  compName: string,
  infoType: string,
  startTime: number,
  endTime: number,
  numDots = 40,
  // How reasons are grouped server-side: "kind" or the finer "kind-what" (matches
  // the client's colorMode so the bands resolve to legend colors).
  group = "kind-what",
) {
  const [info, setInfo] = useState<StackedComponentInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastKeyRef = useRef(`${compName}\n${infoType}`);

  useEffect(() => {
    // Only blank when the component/metric changes; a range/bin jitter keeps the
    // previous chart while the new one loads (no flicker between progressive
    // passes).
    const key = `${compName}\n${infoType}`;
    if (lastKeyRef.current !== key) {
      lastKeyRef.current = key;
      setInfo(null);
    }
    if (!compName || !infoType || infoType === "-" || startTime >= endTime) {
      return;
    }

    const controller = new AbortController();
    setLoading(true);
    setError(null);

    // A coarse 1-in-N pass paints fast, a denser pass sharpens it, then an exact
    // pass corrects it. The deterministic rowid stride badly under-represents a
    // small scope — it may catch only one of a handful of tasks (or none), so the
    // bands can be blank or show the wrong single reason — and the blocking chart
    // over a small scope is cheap to compute exactly. The server declines an exact
    // scan for a scope too large to afford (it returns no rows); in that case we
    // keep the sampled result rather than blanking the chart.
    const schedule = [128, 8];
    let firstDone = false;
    let sampledHadData = false;

    const commit = (json: StackedComponentInfo) => {
      setInfo(json);
      if (!firstDone) {
        firstDone = true;
        setLoading(false);
      }
    };

    const fetchPass = async (sample: number): Promise<StackedComponentInfo | null> => {
      const params = new URLSearchParams({
        // The stacked "ConcurrentTaskMilestones" metric is scope-aware on the
        // backend, so the scoped detail view aggregates a whole subtree.
        scope: compName,
        info_type: infoType,
        start_time: String(startTime),
        end_time: String(endTime),
        num_dots: String(numDots),
        group,
      });
      if (sample > 1) params.set("sample", String(sample));
      const response = await fetch(`/api/compinfo?${params.toString()}`, {
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const json: StackedComponentInfo = await response.json();
      if (controller.signal.aborted) return null;

      return json;
    };

    void (async () => {
      try {
        for (const sample of schedule) {
          const json = await fetchPass(sample);
          if (json === null) return;
          if ((json.kinds?.length ?? 0) > 0) sampledHadData = true;
          commit(json);
        }
        // Keep the exact result only if it has data, or if the sample was also
        // empty (a genuinely empty scope). An empty exact result while the sample
        // had data means the server declined exact for a too-large scope.
        const exact = await fetchPass(1);
        if (exact === null) return;
        if ((exact.kinds?.length ?? 0) > 0 || !sampledHadData) {
          commit(exact);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      }
    })();

    return () => controller.abort();
  }, [compName, infoType, startTime, endTime, numDots, group]);

  useRenderReady(loading, error !== null);

  return { info, loading, error };
}
