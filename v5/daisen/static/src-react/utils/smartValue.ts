interface Tier {
  val: number;
  letter: string;
}

const allTiers: Tier[] = [
  { val: 1e12, letter: "T" },
  { val: 1e9, letter: "G" },
  { val: 1e6, letter: "M" },
  { val: 1e3, letter: "K" },
  { val: 1, letter: "" },
  { val: 1e-3, letter: "m" },
  { val: 1e-6, letter: "μ" },
  { val: 1e-9, letter: "n" },
  { val: 1e-12, letter: "p" },
];

export function smartString(val: number): string {
  for (const t of allTiers) {
    if (val > t.val) {
      return (val / t.val).toFixed(2) + t.letter;
    }
  }

  if (val == undefined) {
    return "undefined";
  }

  return val.toExponential(10);
}
