interface Tier {
  val: number;
  label: string;
}

// Time values are in picoseconds (uint64)
// 1 ps = 1 (input value 1)
// 1 ns = 1_000 ps
// 1 μs = 1_000_000 ps
// 1 ms = 1_000_000_000 ps
// 1 s  = 1_000_000_000_000 ps
const timeTiers: Tier[] = [
  { val: 1e12, label: 's' },
  { val: 1e9,  label: 'ms' },
  { val: 1e6,  label: 'μs' },
  { val: 1e3,  label: 'ns' },
  { val: 1,    label: 'ps' },
];

export function smartTimeString(ps: number): string {
  for (const t of timeTiers) {
    if (ps >= t.val) {
      return (ps / t.val).toFixed(2) + t.label;
    }
  }
  return ps.toFixed(0) + 'ps';
}

const genericTiers: Tier[] = [
  { val: 1e12, label: 'T' },
  { val: 1e9,  label: 'G' },
  { val: 1e6,  label: 'M' },
  { val: 1e3,  label: 'K' },
  { val: 1,    label: '' },
];

export function smartString(val: number): string {
  for (const t of genericTiers) {
    if (val >= t.val) {
      return (val / t.val).toFixed(2) + t.label;
    }
  }
  if (val === 0) return '0';
  return val.toExponential(2);
}
