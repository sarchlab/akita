import { useEffect, useState } from "react";

interface ComponentNamesState {
  names: string[];
  loading: boolean;
  error: string | null;
}

/**
 * Fetch component names from /api/compnames.
 */
export function useComponentNames(): ComponentNamesState {
  const [names, setNames] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    fetch("/api/compnames", { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json: string[]) => {
        setNames(json ?? []);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, []);

  return { names, loading, error };
}
