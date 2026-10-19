import { format } from "d3";

// d3's SI formatter, trimming insignificant trailing zeros.
const si = format("~s");

// formatSI renders a number with SI unit prefixes (…n, u, m, k, M, G…) for compact
// axis labels — e.g. 0.00006 -> "60u", 0.05 -> "50m", 10000 -> "10k", 2e6 -> "2M" —
// instead of scientific notation. It uses an ASCII "u" for micro rather than d3's
// "µ" so it reads cleanly in a terminal/plain font.
export function formatSI(value: number): string {
  if (!Number.isFinite(value)) return "-";
  return si(value).replace("µ", "u");
}
