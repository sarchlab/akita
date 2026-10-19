import { useEffect, useRef } from "react";

/**
 * On narrow windows the detail pane sits below the list: bring it into view
 * when the selection changes (only below `maxWidth`, where it is not beside the list).
 */
export function useScrollIntoView<T extends HTMLElement>(dep: unknown, maxWidth: number) {
  const ref = useRef<T>(null);
  const first = useRef(true);
  useEffect(() => {
    if (first.current) { first.current = false; return; }
    if (dep && ref.current && window.innerWidth < maxWidth) ref.current.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [dep, maxWidth]);
  return ref;
}
