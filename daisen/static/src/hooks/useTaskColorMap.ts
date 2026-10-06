import { useMemo } from "react";
import type { Task } from "../types/task";
import {
  buildColorMapFromKeys,
  taskColorKey,
  type ColorMap,
  type ColorMode,
  type Palette,
} from "../utils/taskColorCoder";

export function useColorMapFromKeys(keys: readonly string[], palette: Palette = "task"): ColorMap {
  return useMemo(() => buildColorMapFromKeys([...keys], palette), [keys, palette]);
}

export function useTaskColorMap(
  tasks: readonly Pick<Task, "kind" | "what">[],
  mode: ColorMode = "kind-what",
  palette: Palette = "task",
): ColorMap {
  return useMemo(
    () => buildColorMapFromKeys(tasks.map((task) => taskColorKey(task, mode)), palette),
    [tasks, mode, palette],
  );
}
