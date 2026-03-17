import { useEffect, useState } from "react";
import type { GitHubIsAvailableResponse } from "../types/chat";

interface GitHubStatus {
  available: boolean;
  routineKeys: string[];
  loading: boolean;
}

export function useGitHubStatus(): GitHubStatus {
  const [available, setAvailable] = useState(false);
  const [routineKeys, setRoutineKeys] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();

    const fetchStatus = async () => {
      try {
        const response = await fetch("/api/githubisavailable", {
          method: "GET",
          signal: controller.signal,
        });

        if (!response.ok) {
          setAvailable(false);
          setRoutineKeys([]);
          return;
        }

        const data = (await response.json()) as Partial<GitHubIsAvailableResponse>;
        const nextKeys = Array.isArray(data.routine_keys) ? [...data.routine_keys].sort() : [];

        setAvailable(data.available === 1);
        setRoutineKeys(nextKeys);
      } catch (err: unknown) {
        if (err instanceof DOMException && err.name === "AbortError") return;

        setAvailable(false);
        setRoutineKeys([]);
      } finally {
        setLoading(false);
      }
    };

    void fetchStatus();

    return () => controller.abort();
  }, []);

  return { available, routineKeys, loading };
}
