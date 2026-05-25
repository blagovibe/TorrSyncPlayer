import { Buffer } from "buffer";

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
        throw new Error("MD5 is not supported by Web Crypto API");
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
  const bytes = new Uint8Array(size);
  getWebCrypto().getRandomValues(bytes);
  return Buffer.from(bytes);
}

function randomFillSync(buffer: Uint8Array | Buffer, offset?: number, size?: number): Uint8Array | Buffer {
  const start = offset ?? 0;
  const length = size ?? buffer.length - start;
  const view = new Uint8Array(buffer.buffer, buffer.byteOffset + start, length);
  getWebCrypto().getRandomValues(view);
  return buffer;
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
  constants: {
    OPENSSL_VERSION_NUMBER: 0,
    SSL_OP_ALL: 0,
    SSL_OP_ALLOW_UNSAFE_LEGACY_RENEGOTIATION: 0,
    SSL_OP_CIPHER_SERVER_PREFERENCE: 0,
    SSL_OP_CISCO_ANYCONNECT: 0,
    SSL_OP_COOKIE_EXCHANGE: 0,
    SSL_OP_CRYPTOPRO_TLSEXT_BUG: 0,
    SSL_OP_DONT_INSERT_EMPTY_FRAGMENTS: 0,
    SSL_OP_EPHEMERAL_RSA: 0,
    SSL_OP_LEGACY_SERVER_CONNECT: 0,
    SSL_OP_MICROSOFT_BIG_SSLV3_BUFFER: 0,
    SSL_OP_MICROSOFT_SESS_ID_BUG: 0,
    SSL_OP_MSIE_SSLV2_RSA_PADDING: 0,
    SSL_OP_NETSCAPE_CA_DN_BUG: 0,
    SSL_OP_NETSCAPE_CHALLENGE_BUG: 0,
    SSL_OP_NETSCAPE_DEMO_CIPHER_CHANGE_BUG: 0,
    SSL_OP_NETSCAPE_REUSE_CIPHER_CHANGE_BUG: 0,
    SSL_OP_NO_COMPRESSION: 0,
    SSL_OP_NO_QUERY_MTU: 0,
    SSL_OP_NO_SESSION_RESUMPTION_ON_RENEGOTIATION: 0,
    SSL_OP_NO_SSLv2: 0,
    SSL_OP_NO_SSLv3: 0,
    SSL_OP_NO_TICKET: 0,
    SSL_OP_NO_TLSv1: 0,
    SSL_OP_NO_TLSv1_1: 0,
    SSL_OP_NO_TLSv1_2: 0,
    SSL_OP_PKCS1_CHECK_1: 0,
    SSL_OP_PKCS1_CHECK_2: 0,
    SSL_OP_SINGLE_DH_USE: 0,
    SSL_OP_SINGLE_ECDH_USE: 0,
    SSL_OP_SSLEAY_080_CLIENT_DH_BUG: 0,
    SSL_OP_SSLREF2_REUSE_CERT_TYPE_BUG: 0,
    SSL_OP_TLSEXT_PADDING: 0,
    SSL_OP_TLS_BLOCK_PADDING_BUG: 0,
    SSL_OP_TLS_D5_BUG: 0,
    SSL_OP_TLS_ROLLBACK_BUG: 0,
    ENGINE_METHOD_ALL: 0,
    ENGINE_METHOD_NONE: 0,
    DH_CHECK_P_NOT_SAFE_PRIME: 0,
    DH_CHECK_P_NOT_PRIME: 0,
    DH_UNABLE_TO_CHECK_GENERATOR: 0,
    DH_NOT_SUITABLE_GENERATOR: 0,
    ALPN_ENABLED: 0,
    RSA_PKCS1_PADDING: 1,
    RSA_SSLV23_PADDING: 2,
    RSA_NO_PADDING: 3,
    RSA_PKCS1_OAEP_PADDING: 4,
    RSA_X931_PADDING: 5,
    RSA_PKCS1_PSS_PADDING: 6,
    POINT_CONVERSION_COMPRESSED: 2,
    POINT_CONVERSION_UNCOMPRESSED: 4,
    POINT_CONVERSION_HYBRID: 6,
  },
};

export default crypto;
export {
  createHashAsync as createHash,
  createHmacAsync as createHmac,
  randomBytes,
  randomFillSync,
  pbkdf2,
  pbkdf2Sync,
  timingSafeEqual,
  getHashes,
  getCiphers,
};
