const SIZE_UNITS = ["B", "KiB", "MiB", "GiB", "TiB"] as const;
const SPEED_UNITS = ["B/s", "KiB/s", "MiB/s", "GiB/s"] as const;

function formatUnit(value: number, units: readonly string[]): string {
  if (!Number.isFinite(value) || value <= 0) return `0 ${units[0]}`;
  let v = value;
  let unitIndex = 0;
  while (v >= 1024 && unitIndex < units.length - 1) {
    v /= 1024;
    unitIndex += 1;
  }
  const precision = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return `${parseFloat(v.toFixed(precision))} ${units[unitIndex]}`;
}

export function formatBytes(size: number): string {
  return formatUnit(size, SIZE_UNITS);
}

export function formatSpeed(bytesPerSecond: number): string {
  return formatUnit(bytesPerSecond, SPEED_UNITS);
}
