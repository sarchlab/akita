import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";
import { beginRenderWork, markNavigation } from "../utils/renderReady.mjs";

/**
 * Contribute a data hook's in-flight state to the global render-ready signal.
 * Begins tracking when `active` (e.g. the hook's `loading`) turns true, and ends
 * — carrying the latest `errored` flag — when it turns false or the component
 * unmounts. See utils/renderReady.mjs.
 */
export function useRenderReady(active: boolean, errored = false): void {
  const erroredRef = useRef(errored);
  erroredRef.current = errored;

  useEffect(() => {
    if (!active) return undefined;
    const end = beginRenderWork();
    return () => end({ errored: erroredRef.current });
  }, [active]);
}

/**
 * Reset the render-ready signal whenever the route/URL changes, so a freshly
 * navigated view is not reported "ready" using the previous view's state.
 * Mount once near the app root (Layout).
 */
export function useRenderReadyOnNavigation(): void {
  const location = useLocation();
  useEffect(() => {
    markNavigation();
  }, [location.key]);
}
