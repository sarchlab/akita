import { useEffect, useState } from "react";

interface EngineTimeState {
  now: string | null;
  loading: boolean;
  error: string | null;
}

/**
 * Polls /api/now every `intervalMs` milliseconds (default 1 000)
 * and returns the current simulation time.
 */
export function useEngineTime(intervalMs = 1000): EngineTimeState {
  const [now, setNow] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    const fetchTime = () => {
      fetch("/api/now", { signal: controller.signal })
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.json();
        })
        .then((data: { now: string }) => {
          setNow(data.now);
          setLoading(false);
          setError(null);
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setError(err instanceof Error ? err.message : String(err));
          setLoading(false);
        });
    };

    fetchTime();
    const id = window.setInterval(fetchTime, intervalMs);

    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  }, [intervalMs]);

  return { now, loading, error };
}
