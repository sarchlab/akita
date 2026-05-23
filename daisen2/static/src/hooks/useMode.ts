import { useEffect, useState } from "react";
import { parseModeResponse } from "../utils/mode.mjs";

export type DaisenMode = "live" | "replay";

export function useMode() {
  const [mode, setMode] = useState<DaisenMode | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/mode")
      .then((response) => response.text())
      .then((text) => {
        if (!cancelled) setMode(parseModeResponse(text));
      })
      .catch(() => {
        if (!cancelled) setMode(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { mode, loading };
}
