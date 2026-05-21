const MemoryChunkStore = require("memory-chunk-store");

class BoundedChunkStore {
  constructor(chunkLength, opts) {
    this.baseStore = new MemoryChunkStore(chunkLength, opts);
    this.chunkLength = chunkLength;
    this.maxBytes = (opts && opts.maxBytes) || Infinity;
    this.currentBytes = 0;
    this.chunks = new Map();
    this.lruList = new Set();
    this.closed = false;
    this.lastChunkLength = opts && opts.length
      ? opts.length % chunkLength || chunkLength
      : chunkLength;
    this.lastChunkIndex = opts && opts.length
      ? Math.ceil(opts.length / chunkLength) - 1
      : Infinity;
  }

  get length() {
    return this.currentBytes;
  }

  put(index, buf, cb) {
    if (this.closed) {
      queueMicrotask(() => cb(new Error("Storage is closed")));
      return;
    }

    const isLastChunk = index === this.lastChunkIndex;
    const expectedLength = isLastChunk ? this.lastChunkLength : this.chunkLength;
    if (buf.length !== expectedLength) {
      queueMicrotask(() => cb(new Error(`Chunk length must be ${expectedLength}`)));
      return;
    }

    const existing = this.chunks.get(index);
    if (existing) {
      this.currentBytes -= existing.length;
      this.lruList.delete(index);
    }

    while (this.currentBytes + buf.length > this.maxBytes && this.lruList.size > 0) {
      const evictIndex = this._findEvictableIndex(index);
      if (evictIndex === null) break;
      this._evictChunk(evictIndex);
    }

    this.chunks.set(index, buf);
    this.lruList.add(index);
    this.currentBytes += buf.length;

    this.baseStore.put(index, buf, cb);
  }

  get(index, opts, cb) {
    if (typeof opts === "function") {
      return this.get(index, null, opts);
    }
    if (this.closed) {
      queueMicrotask(() => cb(new Error("Storage is closed")));
      return;
    }

    if (this.lruList.has(index)) {
      this.lruList.delete(index);
      this.lruList.add(index);
    }

    this.baseStore.get(index, opts || null, cb);
  }

  close(cb) {
    this.destroy(cb);
  }

  destroy(cb) {
    if (this.closed) {
      queueMicrotask(() => cb(new Error("Storage is closed")));
      return;
    }
    this.closed = true;
    this.chunks.clear();
    this.lruList.clear();
    this.currentBytes = 0;
    this.baseStore.destroy(cb);
  }

  _findEvictableIndex(currentIndex) {
    for (const idx of this.lruList) {
      if (idx !== currentIndex) {
        return idx;
      }
    }
    return null;
  }

  _evictChunk(index) {
    const chunk = this.chunks.get(index);
    if (chunk) {
      this.chunks.delete(index);
      this.currentBytes -= chunk.length;
      this.lruList.delete(index);
    }
  }
}

module.exports = { BoundedChunkStore };
