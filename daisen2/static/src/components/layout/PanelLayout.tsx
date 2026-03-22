import { useCallback, useEffect, useRef, useState } from "react";
import ResizableDivider from "./ResizableDivider";

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

const DEFAULT_LEFT_WIDTH = 200;
const DEFAULT_RIGHT_WIDTH = 700;
const DEFAULT_BOTTOM_HEIGHT = 0;
const DIVIDER_SIZE = 8;
const MIN_PANEL_WIDTH = 100;
const MIN_CENTER_WIDTH = 200;
const MIN_BOTTOM_HEIGHT = 0;
const MAX_BOTTOM_RATIO = 0.5;

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface PanelLayoutProps {
  /** Content for the left panel (e.g. component tree). */
  left: React.ReactNode;
  /** Content for the center panel (e.g. detail view). */
  center: React.ReactNode;
  /** Content for the right panel (e.g. tools). */
  right: React.ReactNode;
  /** Content for the bottom panel (e.g. monitor widgets). */
  bottom?: React.ReactNode;
  /** Whether the bottom panel is visible. */
  showBottom?: boolean;
  /** Height available for the layout (excluding the navbar). */
  navBarHeight?: number;
}

/* ------------------------------------------------------------------ */
/*  PanelLayout                                                        */
/* ------------------------------------------------------------------ */

/**
 * PanelLayout — three-pane resizable layout with optional bottom panel.
 *
 * Ported from v5/monitoring/web/src/ui_manager.ts.
 *
 * Layout:
 *   ┌──────┬──────────────┬────────┐
 *   │ left │   center     │ right  │
 *   │      │              │        │
 *   ├──────┴──────────────┴────────┤  ← horizontal divider (if bottom)
 *   │         bottom               │
 *   └──────────────────────────────┘
 *
 * Vertical dividers between left/center and center/right are draggable.
 * Horizontal divider above bottom panel is draggable when bottom is shown.
 * Panel sizes persist for the session via state.
 */
export default function PanelLayout({
  left,
  center,
  right,
  bottom,
  showBottom = false,
  navBarHeight = 56,
}: PanelLayoutProps) {
  /* ------ sizes --------------------------------------------------- */
  const [leftWidth, setLeftWidth] = useState(DEFAULT_LEFT_WIDTH);
  const [rightWidth, setRightWidth] = useState(DEFAULT_RIGHT_WIDTH);
  const [bottomHeight, setBottomHeight] = useState(DEFAULT_BOTTOM_HEIGHT);

  /* Use a ref for window width to avoid full re-renders on resize */
  const containerRef = useRef<HTMLDivElement>(null);

  /* ------ window resize ------------------------------------------- */
  useEffect(() => {
    const handleResize = () => {
      /* Clamp panels so they don't exceed window width */
      const ww = window.innerWidth;
      const maxLeft = ww - MIN_CENTER_WIDTH - rightWidth - DIVIDER_SIZE * 2;
      if (leftWidth > maxLeft) setLeftWidth(Math.max(MIN_PANEL_WIDTH, maxLeft));

      const maxRight = ww - MIN_CENTER_WIDTH - leftWidth - DIVIDER_SIZE * 2;
      if (rightWidth > maxRight)
        setRightWidth(Math.max(MIN_PANEL_WIDTH, maxRight));
    };

    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, [leftWidth, rightWidth]);

  /* Auto-pop bottom panel when showBottom toggles on */
  useEffect(() => {
    if (showBottom && bottomHeight === 0) {
      setBottomHeight(200);
    }
    if (!showBottom) {
      setBottomHeight(0);
    }
  }, [showBottom]); // eslint-disable-line react-hooks/exhaustive-deps

  /* ------ drag callbacks ------------------------------------------ */
  const onDragLeft = useCallback(
    (clientX: number) => {
      const newLeft = clientX - DIVIDER_SIZE / 2;
      const ww = window.innerWidth;
      const maxLeft = ww - MIN_CENTER_WIDTH - rightWidth - DIVIDER_SIZE * 2;
      setLeftWidth(Math.max(MIN_PANEL_WIDTH, Math.min(newLeft, maxLeft)));
    },
    [rightWidth],
  );

  const onDragRight = useCallback(
    (clientX: number) => {
      const ww = window.innerWidth;
      const newRight = ww - clientX - DIVIDER_SIZE / 2;
      const maxRight = ww - MIN_CENTER_WIDTH - leftWidth - DIVIDER_SIZE * 2;
      setRightWidth(Math.max(MIN_PANEL_WIDTH, Math.min(newRight, maxRight)));
    },
    [leftWidth],
  );

  const onDragBottom = useCallback(
    (clientY: number) => {
      const totalHeight = window.innerHeight - navBarHeight;
      const mainHeight = clientY - navBarHeight - DIVIDER_SIZE / 2;
      const newBottom = totalHeight - mainHeight - DIVIDER_SIZE;
      const maxBottom = totalHeight * MAX_BOTTOM_RATIO;
      setBottomHeight(
        Math.max(MIN_BOTTOM_HEIGHT, Math.min(newBottom, maxBottom)),
      );
    },
    [navBarHeight],
  );

  /* ------ derived dimensions -------------------------------------- */
  const totalHeight = `calc(100vh - ${navBarHeight}px)`;
  const bottomSection = showBottom ? bottomHeight + DIVIDER_SIZE : 0;
  const mainHeight = `calc(100vh - ${navBarHeight + bottomSection}px)`;
  const centerWidth = `calc(100% - ${leftWidth + rightWidth + DIVIDER_SIZE * 2}px)`;

  return (
    <div
      ref={containerRef}
      style={{
        height: totalHeight,
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
      }}
    >
      {/* ── Top area: three-pane ───────────────────────────── */}
      <div
        style={{
          height: mainHeight,
          display: "flex",
          flexDirection: "row",
          overflow: "hidden",
          flexShrink: 0,
        }}
      >
        {/* Left panel */}
        <div
          style={{
            width: leftWidth,
            height: "100%",
            overflow: "auto",
            flexShrink: 0,
          }}
        >
          {left}
        </div>

        {/* Left divider */}
        <ResizableDivider
          orientation="vertical"
          size={DIVIDER_SIZE}
          onDrag={onDragLeft}
        />

        {/* Center panel */}
        <div
          style={{
            width: centerWidth,
            height: "100%",
            overflow: "auto",
            flexGrow: 1,
            flexShrink: 1,
            minWidth: MIN_CENTER_WIDTH,
          }}
        >
          {center}
        </div>

        {/* Right divider */}
        <ResizableDivider
          orientation="vertical"
          size={DIVIDER_SIZE}
          onDrag={onDragRight}
        />

        {/* Right panel */}
        <div
          style={{
            width: rightWidth,
            height: "100%",
            overflow: "auto",
            flexShrink: 0,
          }}
        >
          {right}
        </div>
      </div>

      {/* ── Bottom area: monitor widgets ───────────────────── */}
      {showBottom && (
        <>
          <ResizableDivider
            orientation="horizontal"
            size={DIVIDER_SIZE}
            onDrag={onDragBottom}
          />
          <div
            style={{
              height: bottomHeight,
              overflow: "auto",
              flexShrink: 0,
            }}
          >
            {bottom}
          </div>
        </>
      )}
    </div>
  );
}
