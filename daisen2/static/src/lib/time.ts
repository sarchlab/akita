// formatVirtualTime renders an Akita virtual-time value (picoseconds) in the
// largest unit that keeps it readable.
export function formatVirtualTime(ps: number): string {
  if (!Number.isFinite(ps)) return "—";
  const units: [number, string][] = [
    [1e12, "s"],
    [1e9, "ms"],
    [1e6, "µs"],
    [1e3, "ns"],
    [1, "ps"],
  ];
  for (const [scale, unit] of units) {
    if (Math.abs(ps) >= scale) {
      const v = ps / scale;
      return `${v.toLocaleString(undefined, { maximumFractionDigits: 3 })} ${unit}`;
    }
  }
  return `${ps} ps`;
}
