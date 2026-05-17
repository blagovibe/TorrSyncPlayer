/**
 * Retry with exponential backoff.
 *
 * Inspired by multiplex's reconnection strategy.
 *
 * Usage:
 * ```ts
 * const result = await withRetry(
 *   () => p2pService.connect(peerId),
 *   { maxAttempts: 5, baseDelayMs: 1000, maxDelayMs: 30000 }
 * );
 * ```
 */

export interface RetryOptions {
  maxAttempts: number;
  baseDelayMs: number;
  maxDelayMs: number;
  /** Optional predicate to determine if error is retryable */
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

      if (isRetryable && !isRetryable(error)) {
        throw error;
      }

      if (currentAttempt === maxAttempts) {
        throw lastError;
      }

      const delay = Math.min(
        baseDelayMs * Math.pow(2, currentAttempt - 1),
        maxDelayMs,
      );
      return new Promise<T>((resolve) => {
        setTimeout(() => resolve(attempt(currentAttempt + 1)), delay);
      });
    });
  };

  return attempt(1);
}

/**
 * Calculate the next retry delay with exponential backoff and jitter.
 */
export function getBackoffDelay(
  attempt: number,
  baseDelayMs: number,
  maxDelayMs: number,
): number {
  const exponential = baseDelayMs * Math.pow(2, attempt - 1);
  const jitter = exponential * 0.2 * Math.random();
  return Math.min(exponential + jitter, maxDelayMs);
}
