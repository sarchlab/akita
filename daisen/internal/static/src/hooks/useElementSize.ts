import { useCallback, useRef, useState } from "react";

interface Size {
  width: number;
  height: number;
}

/** Observes an element and reports its pixel size, for sizing canvases/charts. */
export function useElementSize<T extends HTMLElement>(initialSize: Size = { width: 600, height: 400 }) {
  const observerRef = useRef<ResizeObserver | null>(null);
  const elementRef = useRef<T | null>(null);
  const [size, setSize] = useState(initialSize);

  const ref = useCallback((node: T | null) => {
    observerRef.current?.disconnect();
    elementRef.current = node;
    if (!node) return;
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      if (width > 0 && height > 0) setSize({ width, height });
    });
    observer.observe(node);
    observerRef.current = observer;
  }, []);

  return { ref, elementRef, size };
}
