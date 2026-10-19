import { useSyncExternalStore } from "react";

function subscribe(cb: () => void) {
  const obs = new MutationObserver(cb);
  obs.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => obs.disconnect();
}
const get = () => document.documentElement.classList.contains("dark");

/** Whether the page is in dark mode (the .dark class on <html>). */
export function useDark(): boolean {
  return useSyncExternalStore(subscribe, get, () => false);
}
