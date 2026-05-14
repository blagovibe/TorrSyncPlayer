import { Buffer } from "buffer";

type FillableBuffer = Buffer | Uint8Array;

function getWebCrypto(): Crypto {
  const crypto = globalThis.crypto ?? (globalThis as typeof globalThis & { msCrypto?: Crypto }).msCrypto;
  if (!crypto || typeof crypto.getRandomValues !== "function") {
    throw new Error("Secure random number generation is not supported by this browser.");
  }
  return crypto;
}

function assertOffset(offset: number, length: number): void {
  if (typeof offset !== "number" || Number.isNaN(offset)) {
    throw new TypeError("offset must be a number");
  }
  if (offset < 0 || offset > 0xffffffff) {
    throw new TypeError("offset must be a uint32");
  }
  if (offset > length) {
    throw new RangeError("offset out of range");
  }
}

function assertSize(size: number, offset: number, length: number): void {
  if (typeof size !== "number" || Number.isNaN(size)) {
    throw new TypeError("size must be a number");
  }
  if (size < 0 || size > 0xffffffff) {
    throw new TypeError("size must be a uint32");
  }
  if (size + offset > length) {
    throw new RangeError("buffer too small");
  }
}

function fillRandomBytes(buffer: FillableBuffer, offset: number, size: number): FillableBuffer {
  const crypto = getWebCrypto();
  const view = new Uint8Array(buffer.buffer, buffer.byteOffset + offset, size);
  crypto.getRandomValues(view);
  return buffer;
}

export function randomFill(
  buffer: FillableBuffer,
  offset: number | ((error: Error | null, buffer: FillableBuffer) => void) = 0,
  size?: number | ((error: Error | null, buffer: FillableBuffer) => void),
  callback?: (error: Error | null, buffer: FillableBuffer) => void,
): FillableBuffer | void {
  if (!Buffer.isBuffer(buffer) && !(buffer instanceof Uint8Array)) {
    throw new TypeError('"buf" argument must be a Buffer or Uint8Array');
  }

  let start = 0;
  let length = buffer.length;
  let cb = callback;

  if (typeof offset === "function") {
    cb = offset;
  } else {
    start = offset;
    if (typeof size === "function") {
      cb = size;
    } else if (typeof size === "number") {
      length = size;
    } else {
      length = buffer.length - start;
    }
  }

  if (typeof cb !== "function") {
    throw new TypeError('"cb" argument must be a function');
  }

  assertOffset(start, buffer.length);
  assertSize(length, start, buffer.length);

  const filled = fillRandomBytes(buffer, start, length);
  cb(null, filled);
  return;
}

export function randomFillSync(
  buffer: FillableBuffer,
  offset = 0,
  size = buffer.length - offset,
): FillableBuffer {
  if (!Buffer.isBuffer(buffer) && !(buffer instanceof Uint8Array)) {
    throw new TypeError('"buf" argument must be a Buffer or Uint8Array');
  }

  assertOffset(offset, buffer.length);
  assertSize(size, offset, buffer.length);

  return fillRandomBytes(buffer, offset, size);
}

const randomfill = { randomFill, randomFillSync };

export default randomfill;
