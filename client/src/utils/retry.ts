export interface RetryOptions {
  maxAttempts: number;
  baseDelayMs: number;
  maxDelayMs: number;
  isRetryable?: (error: unknown) => boolean;
}

export function withRetry<T>(
  fn: () => Promise<T>,
  options: RetryOptions,
): Promise<T> {
  const { maxAttempts, baseDelayMs, maxDelayMs, isRetryable } = options;
  let lastError: unknown;

  const attempt = (currentAttempt: number): Promise<T> => {
    return fn().catch((error: unknown) => {
      lastError = error;

      if (isRetryable && !isRetryable(error)) throw error;
      if (currentAttempt >= maxAttempts) throw lastError;

      // Cap exponential to prevent overflow: max 2^20 (~1M) * baseDelay
      const cappedAttempt = Math.min(currentAttempt, 20);
      const delay = Math.min(baseDelayMs * Math.pow(2, cappedAttempt - 1), maxDelayMs);
      return new Promise<T>((resolve) => {
        setTimeout(() => resolve(attempt(currentAttempt + 1)), delay);
      });
    });
  };

  return attempt(1);
}

export function getBackoffDelay(
  attempt: number,
  baseDelayMs: number,
  maxDelayMs: number,
): number {
  const cappedAttempt = Math.min(attempt, 20);
  const exponential = baseDelayMs * Math.pow(2, cappedAttempt - 1);
  const jitter = exponential * 0.2 * Math.random();
  return Math.min(exponential + jitter, maxDelayMs);
}
