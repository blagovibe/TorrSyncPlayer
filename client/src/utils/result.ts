/**
 * Result<T, E> pattern for type-safe error handling.
 *
 * Instead of throwing exceptions, functions return Result<T, E>
 * which explicitly encodes success and failure cases.
 *
 * Usage:
 * ```ts
 * function parsePort(input: string): Result<number, string> {
 *   const port = Number(input);
 *   if (!Number.isInteger(port) || port <= 0 || port > 65535) {
 *     return err("Invalid port number");
 *   }
 *   return ok(port);
 * }
 *
 * const result = parsePort("8080");
 * if (result.ok) {
 *   console.log(result.value); // number
 * } else {
 *   console.error(result.error); // string
 * }
 * ```
 */

export type Result<T, E = string> =
  | { ok: true; value: T }
  | { ok: false; error: E };

export function ok<T>(value: T): Result<T, never> {
  return { ok: true, value };
}

export function err<E>(error: E): Result<never, E> {
  return { ok: false, error };
}

/**
 * Unwrap a Result, throwing the error if it's a failure.
 */
export function unwrap<T, E>(result: Result<T, E>): T {
  if (result.ok) {
    return result.value;
  }
  throw new Error(String((result as { error: E }).error));
}

/**
 * Map over a successful Result value.
 */
export function map<T, U, E>(
  result: Result<T, E>,
  fn: (value: T) => U,
): Result<U, E> {
  if (result.ok) {
    return ok(fn(result.value));
  }
  return err((result as { error: E }).error);
}

/**
 * Chain Results together.
 */
export function flatMap<T, U, E>(
  result: Result<T, E>,
  fn: (value: T) => Result<U, E>,
): Result<U, E> {
  if (result.ok) {
    return fn(result.value);
  }
  return err((result as { error: E }).error);
}
