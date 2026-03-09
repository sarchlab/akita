interface Tier {
  val: number;
  letter: string;
}

const allTiers = [
  { val: 1e12, letter: "T" },
  { val: 1e9, letter: "G" },
  { val: 1e6, letter: "M" },
  { val: 1e3, letter: "K" },
  { val: 1, letter: "" },
  { val: 1e-3, letter: "m" },
  { val: 1e-6, letter: "μ" },
  { val: 1e-9, letter: "n" },
  { val: 1e-12, letter: "p" }
];

export function smartString(val: number): string {
  for (let i = 0; i < allTiers.length; i++) {
    const t = allTiers[i];
    if (val > t.val) {
      return (val / t.val).toFixed(2) + t.letter;
    }
  }

  if (val == undefined) {
    console.error("val is undefined");
    return "undefined";
  }

  return val.toExponential(10);
}
