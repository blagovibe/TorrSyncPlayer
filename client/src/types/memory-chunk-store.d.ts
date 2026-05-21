declare module "memory-chunk-store" {
  interface StorageOptions {
    length?: number;
  }

  class Storage {
    constructor(chunkLength: number, opts?: StorageOptions);
    chunkLength: number;
    chunks: Buffer[];
    closed: boolean;
    length: number;

    put(index: number, buf: Buffer, cb?: (err: Error | null) => void): void;
    get(
      index: number,
      opts: { offset?: number; length?: number } | null,
      cb?: (err: Error | null, data?: Buffer) => void
    ): void;
    get(index: number, cb?: (err: Error | null, data?: Buffer) => void): void;
    close(cb?: (err: Error | null) => void): void;
    destroy(cb?: (err: Error | null) => void): void;
  }

  export = Storage;
}
