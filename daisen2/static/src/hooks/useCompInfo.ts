import { useEffect, useState } from "react";
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
      .finally(() => setLoading(false));

    return () => controller.abort();
  }, [compName, infoType, startTime, endTime, numDots]);

  useRenderReady(loading, error !== null);

  return { info, loading, error };
}
