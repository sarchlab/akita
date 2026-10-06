export function smartString(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }

  if (Math.abs(value) < 1e-9) {
    return `${(value * 1e12).toFixed(2)} ps`;
  }
  if (Math.abs(value) < 1e-6) {
    return `${(value * 1e9).toFixed(2)} ns`;
  }
  if (Math.abs(value) < 1e-3) {
    return `${(value * 1e6).toFixed(2)} us`;
  }
  if (Math.abs(value) < 1) {
    return `${(value * 1e3).toFixed(2)} ms`;
  }

  return `${value.toFixed(2)} s`;
}

export function formatPicosecondsAsNanoseconds(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }

  return `${(value / 1000).toLocaleString(undefined, {
    maximumFractionDigits: 3,
  })} ns`;
}
