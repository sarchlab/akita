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

    // Progressive sample (mirrors useComponentTimeline): a coarse 1-in-N pass
    // paints fast, then one denser pass sharpens it. The blocking-reason chart
    // joins the 68M-row milestone table over the scope's tasks, so an exact pass
    // costs minutes on a dense scope — but the deterministic rowid stride can miss
    // every task in a sparse scope, so we fall back to exact only when the sampled
    // passes found nothing (see below).
    const schedule = [128, 8];
    let firstDone = false;
    let lastKinds = 0;

    const runPass = async (sample: number): Promise<boolean> => {
      const params = new URLSearchParams({
        // The stacked "ConcurrentTaskMilestones" metric is scope-aware on the
        // backend, so the scoped detail view aggregates a whole subtree.
        scope: compName,
        info_type: infoType,
        start_time: String(startTime),
        end_time: String(endTime),
        num_dots: String(numDots),
      });
      if (sample > 1) params.set("sample", String(sample));
      const response = await fetch(`/api/compinfo?${params.toString()}`, {
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const json: StackedComponentInfo = await response.json();
      if (controller.signal.aborted) return false;
      setInfo(json);
      lastKinds = json.kinds?.length ?? 0;
      if (!firstDone) {
        firstDone = true;
        setLoading(false);
      }

      return true;
    };

    void (async () => {
      try {
        for (const sample of schedule) {
          if (!(await runPass(sample))) return;
        }
        // Sparse or modulo-skewed scope: the sampled stride matched no tasks, so
        // the bands are blank (or a lone task is overstated ×sample). An exact pass
        // over so few tasks is cheap and recovers the real breakdown.
        if (lastKinds === 0) {
          await runPass(1);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      }
    })();

    return () => controller.abort();
  }, [compName, infoType, startTime, endTime, numDots]);

  useRenderReady(loading, error !== null);

  return { info, loading, error };
}
