import { useEffect, useState } from "react";

export function useGitHubStatus() {
  const [available, setAvailable] = useState(false);
  const [routineKeys, setRoutineKeys] = useState<string[]>([]);

  useEffect(() => {
    fetch("/api/githubisavailable")
      .then((response) => response.json())
      .then((json: { available?: number; routine_keys?: string[] }) => {
        setAvailable(json.available === 1);
        setRoutineKeys(json.routine_keys ?? []);
      })
      .catch(() => {
        setAvailable(false);
        setRoutineKeys([]);
      });
  }, []);

  return { available, routineKeys };
}
