import { useEffect, useRef } from "react";
import type { ColorMode } from "../utils/taskColorCoder";

// useAutoColorMode defaults a coloring toggle to "kind" once "kind-what" would
// produce more than `threshold` distinct keys (too many to tell apart by color).
// The downgrade is one-way and only until the user picks a mode, so it never
// fights the toggle or churns while zooming. Returns the onChange handler to give
// the toggle — it records the user's pick so the auto-downgrade stops.
export function useAutoColorMode(
  mode: ColorMode,
  setMode: (mode: ColorMode) => void,
  keyCount: number,
  threshold: number,
): (mode: ColorMode) => void {
  const userPicked = useRef(false);
  useEffect(() => {
    if (userPicked.current) return;
    if (mode === "kind-what" && keyCount > threshold) setMode("kind");
  }, [mode, keyCount, setMode, threshold]);
  return (next: ColorMode) => {
    userPicked.current = true;
    setMode(next);
  };
}
