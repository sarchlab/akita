import * as d3 from "d3";

export interface Segment {
  start_time: number;
  end_time: number;
}

export interface SegmentsResponse {
  enabled: boolean;
  segments: Segment[];
}

// Cache for segment data to avoid repeated API calls
let cachedSegments: SegmentsResponse | null = null;
let fetchPromise: Promise<SegmentsResponse> | null = null;

/**
 * Fetches segment data from the API. Results are cached.
 */
export async function fetchSegments(): Promise<SegmentsResponse> {
  if (cachedSegments !== null) {
    return cachedSegments;
  }

  if (fetchPromise !== null) {
    return fetchPromise;
  }

  fetchPromise = fetch("/api/segments")
    .then((response) => response.json())
    .then((data: SegmentsResponse) => {
      cachedSegments = data;
      fetchPromise = null;
      return data;
    })
    .catch((error) => {
      console.error("Error fetching segments:", error);
      fetchPromise = null;
      return { enabled: false, segments: [] };
    });

  return fetchPromise;
}

/**
 * Clears the cached segment data (useful for testing or when data might change)
 */
export function clearSegmentCache(): void {
  cachedSegments = null;
  fetchPromise = null;
}

/**
 * Calculates the non-traced periods (gaps between segments) within a given time range.
 * Returns an array of time ranges that should be shaded.
 */
export function calculateNonTracedPeriods(
  segments: Segment[],
  viewStartTime: number,
  viewEndTime: number
): { start: number; end: number }[] {
  if (segments.length === 0) {
    // If no segments, the entire view is non-traced
    return [{ start: viewStartTime, end: viewEndTime }];
  }

  const nonTracedPeriods: { start: number; end: number }[] = [];

  // Sort segments by start time
  const sortedSegments = [...segments].sort((a, b) => a.start_time - b.start_time);

  // Check for gap before first segment
  if (sortedSegments[0].start_time > viewStartTime) {
    nonTracedPeriods.push({
      start: viewStartTime,
      end: Math.min(sortedSegments[0].start_time, viewEndTime),
    });
  }

  // Check for gaps between segments
  for (let i = 0; i < sortedSegments.length - 1; i++) {
    const currentEnd = sortedSegments[i].end_time;
    const nextStart = sortedSegments[i + 1].start_time;

    if (currentEnd < nextStart) {
      // There's a gap between segments
      const gapStart = Math.max(currentEnd, viewStartTime);
      const gapEnd = Math.min(nextStart, viewEndTime);

      if (gapStart < gapEnd) {
        nonTracedPeriods.push({ start: gapStart, end: gapEnd });
      }
    }
  }

  // Check for gap after last segment
  const lastSegment = sortedSegments[sortedSegments.length - 1];
  if (lastSegment.end_time < viewEndTime) {
    nonTracedPeriods.push({
      start: Math.max(lastSegment.end_time, viewStartTime),
      end: viewEndTime,
    });
  }

  return nonTracedPeriods;
}

/**
 * Renders shading overlays on an SVG element for non-traced periods.
 * @param svg The D3 selection of the SVG element
 * @param xScale The D3 scale for mapping time to x coordinates
 * @param nonTracedPeriods Array of time periods to shade
 * @param height The height of the shading rectangles
 * @param yOffset The y offset for the shading rectangles (default 0)
 * @param className CSS class name for the shading group (default "segment-shading")
 */
export function renderSVGShading(
  svg: d3.Selection<SVGSVGElement, unknown, null, undefined>,
  xScale: d3.ScaleLinear<number, number>,
  nonTracedPeriods: { start: number; end: number }[],
  height: number,
  yOffset: number = 0,
  className: string = "segment-shading"
): void {
  // Remove existing shading
  svg.selectAll(`.${className}`).remove();

  if (nonTracedPeriods.length === 0) {
    return;
  }

  const shadingGroup = svg.append("g").attr("class", className);

  // Add diagonal stripe pattern definition if not exists
  let defs = svg.select("defs");
  if (defs.empty()) {
    defs = svg.append("defs");
  }

  const patternId = `${className}-pattern`;
  if (defs.select(`#${patternId}`).empty()) {
    const pattern = defs
      .append("pattern")
      .attr("id", patternId)
      .attr("patternUnits", "userSpaceOnUse")
      .attr("width", 8)
      .attr("height", 8)
      .attr("patternTransform", "rotate(45)");

    pattern
      .append("rect")
      .attr("width", 8)
      .attr("height", 8)
      .attr("fill", "rgba(128, 128, 128, 0.15)");

    pattern
      .append("line")
      .attr("x1", 0)
      .attr("y1", 0)
      .attr("x2", 0)
      .attr("y2", 8)
      .attr("stroke", "rgba(128, 128, 128, 0.3)")
      .attr("stroke-width", 4);
  }

  nonTracedPeriods.forEach((period) => {
    const x = xScale(period.start);
    const width = xScale(period.end) - xScale(period.start);

    if (width > 0) {
      shadingGroup
        .append("rect")
        .attr("x", x)
        .attr("y", yOffset)
        .attr("width", width)
        .attr("height", height)
        .attr("fill", `url(#${patternId})`)
        .attr("pointer-events", "none")
        .append("title")
        .text("Traces not collected during this period");
    }
  });
}

/**
 * Renders shading overlays on a DOM element (div) for non-traced periods.
 * @param container The HTML container element
 * @param xScale The D3 scale or a function for mapping time to x coordinates
 * @param nonTracedPeriods Array of time periods to shade
 * @param height The height of the shading rectangles (CSS value)
 * @param yOffset The top offset for the shading rectangles (CSS value)
 * @param className CSS class name for the shading elements (default "segment-shading-div")
 */
export function renderDOMShading(
  container: HTMLElement,
  xScale: (time: number) => number,
  nonTracedPeriods: { start: number; end: number }[],
  height: string,
  yOffset: string = "0",
  className: string = "segment-shading-div"
): void {
  // Remove existing shading
  const existing = container.querySelectorAll(`.${className}`);
  existing.forEach((el) => el.remove());

  if (nonTracedPeriods.length === 0) {
    return;
  }

  nonTracedPeriods.forEach((period) => {
    const x = xScale(period.start);
    const width = xScale(period.end) - xScale(period.start);

    if (width > 0) {
      const shadingDiv = document.createElement("div");
      shadingDiv.className = className;
      shadingDiv.title = "Traces not collected during this period";
      shadingDiv.style.cssText = `
        position: absolute;
        left: ${x}px;
        top: ${yOffset};
        width: ${width}px;
        height: ${height};
        background: repeating-linear-gradient(
          45deg,
          rgba(128, 128, 128, 0.15),
          rgba(128, 128, 128, 0.15) 2px,
          rgba(128, 128, 128, 0.25) 2px,
          rgba(128, 128, 128, 0.25) 4px
        );
        pointer-events: none;
        z-index: 5;
      `;
      container.appendChild(shadingDiv);
    }
  });
}

/**
 * Convenience function to fetch segments and render shading on an SVG element.
 */
export async function applySegmentShadingToSVG(
  svg: d3.Selection<SVGSVGElement, unknown, null, undefined>,
  xScale: d3.ScaleLinear<number, number>,
  viewStartTime: number,
  viewEndTime: number,
  height: number,
  yOffset: number = 0,
  className: string = "segment-shading"
): Promise<void> {
  const segmentData = await fetchSegments();

  if (!segmentData.enabled) {
    // Remove any existing shading if feature is disabled
    svg.selectAll(`.${className}`).remove();
    return;
  }

  const nonTracedPeriods = calculateNonTracedPeriods(
    segmentData.segments,
    viewStartTime,
    viewEndTime
  );

  renderSVGShading(svg, xScale, nonTracedPeriods, height, yOffset, className);
}

/**
 * Convenience function to fetch segments and render shading on a DOM element.
 */
export async function applySegmentShadingToDOM(
  container: HTMLElement,
  xScale: (time: number) => number,
  viewStartTime: number,
  viewEndTime: number,
  height: string,
  yOffset: string = "0",
  className: string = "segment-shading-div"
): Promise<void> {
  const segmentData = await fetchSegments();

  if (!segmentData.enabled) {
    // Remove any existing shading if feature is disabled
    const existing = container.querySelectorAll(`.${className}`);
    existing.forEach((el) => el.remove());
    return;
  }

  const nonTracedPeriods = calculateNonTracedPeriods(
    segmentData.segments,
    viewStartTime,
    viewEndTime
  );

  renderDOMShading(container, xScale, nonTracedPeriods, height, yOffset, className);
}
