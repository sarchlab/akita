// LoadingCurve is a placeholder silhouette shown while a chart's occupancy data
// is still loading (those queries take a while on a large scope). It is a
// deterministic mock density shape — not real data — drawn in muted gray with a
// bright highlight stripe that sweeps left→right (skeleton-shimmer style) so the
// panel clearly reads as "loading" rather than sitting blank. `id` must be
// unique per instance on the page (the clip path / gradient are referenced by id).
export default function LoadingCurve({
  width,
  height,
  id,
}: {
  width: number;
  height: number;
  id: string;
}) {
  const w = Math.max(1, width);
  const h = Math.max(1, height);
  const n = 96;
  // A per-instance phase derived from the id, so the two charts' mock curves
  // look different rather than identical twins. Deterministic (no Math.random)
  // so the shape stays stable across re-renders instead of reshuffling.
  let seed = 0;
  for (let i = 0; i < id.length; i++) seed += id.charCodeAt(i) * (i + 1);
  const ph = ((seed % 100) / 100) * Math.PI * 2;
  const pts: string[] = [];
  for (let i = 0; i <= n; i++) {
    const t = i / n;
    // An irregular, lopsided density profile: a few non-harmonic sine components
    // (so it never reads as periodic or mirror-symmetric) under a soft envelope
    // that lifts it off the baseline and brings it back down at both ends.
    const bumps =
      0.5 +
      0.22 * Math.sin(t * 6.0 + 0.6 + ph) +
      0.13 * Math.sin(t * 11.3 + 2.1 + ph * 1.7) +
      0.08 * Math.sin(t * 19.7 + 4.0 + ph * 0.6) +
      0.05 * Math.sin(t * 31.1 + 1.2 + ph * 2.3);
    const envelope = Math.pow(Math.sin(Math.PI * t), 0.35);
    const frac = Math.min(1, Math.max(0, 0.06 + 0.92 * Math.max(0, bumps) * envelope));
    const x = 5 + t * (w - 10);
    const y = h - 4 - frac * (h - 12);
    pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
  }
  const d = `M${pts.join("L")}L${(w - 5).toFixed(1)},${h} L5,${h} Z`;
  const clipId = `lc-clip-${id}`;
  const gradId = `lc-grad-${id}`;
  return (
    <g pointerEvents="none">
      <defs>
        <clipPath id={clipId}>
          <path d={d} />
        </clipPath>
        {/* A wide, soft, low-contrast highlight band — translating the rect that
            carries it glides the band across the silhouette left→right, like the
            skeleton shimmer shown while images load on the web. The base gray
            stays steady; only this lighter band moves. */}
        <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#fff" stopOpacity="0" />
          <stop offset="25%" stopColor="#fff" stopOpacity="0" />
          <stop offset="50%" stopColor="#fff" stopOpacity="0.55" />
          <stop offset="75%" stopColor="#fff" stopOpacity="0" />
          <stop offset="100%" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={d} fill="#cbd5e1" />
      <g clipPath={`url(#${clipId})`}>
        {/* The band enters just off the left edge and exits just off the right,
            so the loop restart lands off-screen and the sweep reads as one
            continuous, gently easing motion. */}
        <rect x={0} y={0} width={w} height={h} fill={`url(#${gradId})`}>
          <animate
            attributeName="x"
            from={-0.85 * w}
            to={0.85 * w}
            dur="1.8s"
            calcMode="spline"
            keyTimes="0;1"
            keySplines="0.4 0 0.6 1"
            repeatCount="indefinite"
          />
        </rect>
      </g>
    </g>
  );
}
