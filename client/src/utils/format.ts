const SIZE_UNITS = ["B", "KB", "MB", "GB", "TB"] as const;
const SPEED_UNITS = ["B/s", "KB/s", "MB/s", "GB/s"] as const;

export function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return "0 B";
  }

  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < SIZE_UNITS.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${SIZE_UNITS[unitIndex]}`;
}

export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) {
    return "0 B/s";
  }

  let value = bytesPerSecond;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < SPEED_UNITS.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${SPEED_UNITS[unitIndex]}`;
}
