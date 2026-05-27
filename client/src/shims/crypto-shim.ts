import { Buffer } from "buffer";
import { randomFillSync } from "./randomfill";

type CryptoHash = "sha1" | "sha256" | "sha384" | "sha512" | "md5";

const encoder = new TextEncoder();

function getWebCrypto(): Crypto {
  const crypto = globalThis.crypto ?? (globalThis as typeof globalThis & { msCrypto?: Crypto }).msCrypto;
  if (!crypto || typeof crypto.subtle !== "object") {
    throw new Error("Web Crypto API is not supported by this browser.");
  }
  return crypto;
}

async function digestSHA(hash: "SHA-1" | "SHA-256" | "SHA-384" | "SHA-512", data: Uint8Array): Promise<Buffer> {
  const subtle = getWebCrypto().subtle;
  const hashBuffer = await subtle.digest(hash, data as unknown as BufferSource);
  return Buffer.from(hashBuffer);
}

function createHashAsync(hashName: CryptoHash) {
  const algorithmMap: Record<string, "SHA-1" | "SHA-256" | "SHA-384" | "SHA-512"> = {
    sha1: "SHA-1",
    sha256: "SHA-256",
    sha384: "SHA-384",
    sha512: "SHA-512",
  };

  const algorithm = algorithmMap[hashName];
  let data: Uint8Array | null = null;

  return {
    update(input: string | Uint8Array | Buffer) {
      const newBytes = typeof input === "string"
        ? encoder.encode(input)
        : new Uint8Array(input.buffer ?? input, input.byteOffset ?? 0, input.byteLength ?? input.length);

      if (data === null) {
        data = newBytes;
      } else {
        const combined = new Uint8Array(data.length + newBytes.length);
        combined.set(data);
        combined.set(newBytes, data.length);
        data = combined;
      }
      return this;
    },
    async digest(): Promise<Buffer> {
      if (data === null) {
        data = new Uint8Array(0);
      }

      if (hashName === "md5") {
        throw new Error("MD5 is not supported by Web Crypto API (browser mode). Use the Electron build for full crypto support.");
      }

      return digestSHA(algorithm!, data);
    },
  };
}

async function createHmacAsync(algorithm: CryptoHash, key: string | Buffer | Uint8Array) {
  const algorithmMap: Record<string, "SHA-1" | "SHA-256" | "SHA-384" | "SHA-512"> = {
    sha1: "SHA-1",
    sha256: "SHA-256",
    sha384: "SHA-384",
    sha512: "SHA-512",
  };

  const keyData = typeof key === "string"
    ? encoder.encode(key)
    : new Uint8Array(key.buffer ?? key, key.byteOffset ?? 0, key.byteLength ?? key.length);

  const subtle = getWebCrypto().subtle;
  const cryptoKey = await subtle.importKey(
    "raw",
    keyData as unknown as BufferSource,
    { name: "HMAC", hash: algorithmMap[algorithm] },
    false,
    ["sign"]
  );

  let data: Uint8Array | null = null;

  return {
    update(input: string | Uint8Array | Buffer) {
      const newBytes = typeof input === "string"
        ? encoder.encode(input)
        : new Uint8Array(input.buffer ?? input, input.byteOffset ?? 0, input.byteLength ?? input.length);

      if (data === null) {
        data = newBytes;
      } else {
        const combined = new Uint8Array(data.length + newBytes.length);
        combined.set(data);
        combined.set(newBytes, data.length);
        data = combined;
      }
      return this;
    },
    async digest(): Promise<Buffer> {
      if (data === null) {
        data = new Uint8Array(0);
      }

      const signature = await subtle.sign("HMAC", cryptoKey, data as unknown as BufferSource);
      return Buffer.from(signature);
    },
  };
}

function randomBytes(size: number): Buffer {
  const buf = Buffer.alloc(size);
  randomFillSync(buf);
  return buf;
}

function pbkdf2Sync(): Buffer {
  throw new Error("Synchronous pbkdf2 is not supported. Use pbkdf2() for async version.");
}

async function pbkdf2(
  password: string | Buffer | Uint8Array,
  salt: string | Buffer | Uint8Array,
  iterations: number,
  keylen: number,
  digest: string
): Promise<Buffer> {
  const passwordData = typeof password === "string"
    ? encoder.encode(password)
    : new Uint8Array(password.buffer ?? password, password.byteOffset ?? 0, password.byteLength ?? password.length);

  const saltData = typeof salt === "string"
    ? encoder.encode(salt)
    : new Uint8Array(salt.buffer ?? salt, salt.byteOffset ?? 0, salt.byteLength ?? salt.length);

  const subtle = getWebCrypto().subtle;
  const keyMaterial = await subtle.importKey("raw", passwordData as unknown as BufferSource, "PBKDF2", false, ["deriveBits"]);

  const algorithm: Pbkdf2Params = {
    name: "PBKDF2",
    salt: saltData as unknown as BufferSource,
    iterations,
    hash: digest.toUpperCase(),
  };

  const derivedBits = await subtle.deriveBits(algorithm, keyMaterial, keylen * 8);
  return Buffer.from(derivedBits);
}

function createCipheriv(): never {
  throw new Error("createCipheriv is not supported. Use async cipher functions.");
}

function createDecipheriv(): never {
  throw new Error("createDecipheriv is not supported. Use async decipher functions.");
}

function createECDH(): never {
  throw new Error("createECDH is not supported. Use Web Crypto ECDH directly.");
}

function createSign(): never {
  throw new Error("createSign is not supported. Use Web Crypto sign directly.");
}

function createVerify(): never {
  throw new Error("createVerify is not supported. Use Web Crypto verify directly.");
}

function createDiffieHellman(): never {
  throw new Error("createDiffieHellman is not supported. Use Web Crypto ECDH directly.");
}

function getDiffieHellman(): never {
  throw new Error("getDiffieHellman is not supported. Use Web Crypto ECDH directly.");
}

function getHashes(): string[] {
  return ["sha1", "sha256", "sha384", "sha512"];
}

function getCiphers(): string[] {
  return [];
}

function timingSafeEqual(a: Buffer, b: Buffer): boolean {
  if (a.length !== b.length) {
    throw new Error("Input buffers must have the same length");
  }
  const viewA = new Uint8Array(a.buffer, a.byteOffset, a.length);
  const viewB = new Uint8Array(b.buffer, b.byteOffset, b.length);
  let result = 0;
  for (let i = 0; i < viewA.length; i++) {
    result |= viewA[i] ^ viewB[i];
  }
  return result === 0;
}

const crypto = {
  createHash: createHashAsync,
  createHmac: createHmacAsync,
  randomBytes,
  randomFillSync,
  pbkdf2Sync,
  pbkdf2,
  createCipheriv,
  createDecipheriv,
  createECDH,
  createSign,
  createVerify,
  createDiffieHellman,
  getDiffieHellman,
  getHashes,
  getCiphers,
  timingSafeEqual,
  constants: {},
};

export default crypto;
