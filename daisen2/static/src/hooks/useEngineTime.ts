import { useEffect, useState } from "react";

export function useEngineTime(pollMs = 1000) {
  const [time, setTime] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    const tick = () => {
      fetch("/api/now")
        .then((response) => (response.ok ? response.json() : null))
        .then((json) => {
          if (!cancelled && typeof json?.now === "number") setTime(json.now);
        })
        .catch(() => {});
    };
    tick();
    const id = window.setInterval(tick, pollMs);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [pollMs]);

  return time;
}
