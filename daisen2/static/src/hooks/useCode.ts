import { useEffect, useState } from "react";
import type { CodeLsResponse, CodeReadResponse } from "../types/overview";
import { useRenderReady } from "./useRenderReady";

/** Lists a directory of the recorded source via /api/code/ls. */
export function useCodeLs(path: string) {
  const [data, setData] = useState<CodeLsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetch(`/api/code/ls?path=${encodeURIComponent(path)}`)
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: CodeLsResponse) => {
        if (!cancelled) setData(json);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}

/** Reads a recorded source file via /api/code/read. A null path fetches nothing. */
export function useCodeRead(path: string | null) {
  const [data, setData] = useState<CodeReadResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!path) {
      setData(null);
      setError(null);
      setLoading(false);
      return undefined;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetch(`/api/code/read?path=${encodeURIComponent(path)}`)
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: CodeReadResponse) => {
        if (!cancelled) setData(json);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  return { data, loading, error };
}
