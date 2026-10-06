import { useEffect, useState } from "react";

export function useEngineTime(pollMs = 1000) {
  const [time, setTime] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    let pending = false;
    const controller = new AbortController();
    const tick = () => {
      if (pending) return;
      pending = true;
      fetch("/api/now", { signal: controller.signal })
        .then((response) => (response.ok ? response.json() : null))
        .then((json) => {
          if (!cancelled && typeof json?.now === "number") {
            setTime(json.now);
          }
        })
        .catch(() => {})
        .finally(() => { pending = false; });
    };

    tick();
    const id = window.setInterval(tick, pollMs);
    return () => {
      cancelled = true;
      controller.abort();
      window.clearInterval(id);
    };
  }, [pollMs]);

  return time;
}
