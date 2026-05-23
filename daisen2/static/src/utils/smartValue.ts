export function smartString(value: number): string {
  if (!Number.isFinite(value)) {
    return "-";
  }

  const abs = Math.abs(value);
  if (abs === 0) {
    return "0";
  }
  if (abs < 1e-9) {
    return `${(value * 1e12).toFixed(2)} ps`;
  }
  if (abs < 1e-6) {
    return `${(value * 1e9).toFixed(2)} ns`;
  }
  if (abs < 1e-3) {
    return `${(value * 1e6).toFixed(2)} us`;
  }
  if (abs < 1) {
    return `${(value * 1e3).toFixed(2)} ms`;
  }
  if (abs < 1000) {
    return `${value.toFixed(2)} s`;
  }
  return value.toExponential(2);
}

export const smartTimeString = smartString;
