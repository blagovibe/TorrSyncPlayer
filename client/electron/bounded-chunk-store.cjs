const MemoryChunkStore = require("memory-chunk-store");

/**
 * BoundedChunkStore — LRU-based memory-constrained chunk store.
 *
 * Eviction Strategy:
 * - When `put()` would exceed `maxBytes`, chunks are evicted using a distance-based LRU policy.
 * - The eviction candidate is the chunk farthest from the current write position (`currentIndex`).
 *   This keeps nearby data available for sequential playback while evicting distant chunks first.
 * - LRU re-ordering happens in `get()`: accessing a chunk moves it to the end of the LRU set.
 * - The `lruList` Set maintains insertion order (ES2015+ spec), which doubles as LRU ordering
 *   because `get()` deletes and re-adds entries.
 * - If all chunks are near the current position (no evictable index), the store may temporarily
 *   exceed `maxBytes` until the next sequential write triggers eviction.
 *
 * Note: Uses `Set` iteration for LRU, which is insertion-order dependent. This is intentional —
 * `get()` re-inserts accessed chunks, making the Set act as a simple LRU without a full
 * doubly-linked list implementation.
 */
class BoundedChunkStore {
  constructor(chunkLength, opts) {
    this.baseStore = new MemoryChunkStore(chunkLength, opts);
    this.chunkLength = chunkLength;
    this.maxBytes = (opts && opts.maxBytes) || Infinity;
    this.currentBytes = 0;
    this.chunks = new Map();
    this.lruList = new Set();
    this.closed = false;
    this.lengthKnown = !!(opts && opts.length);
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

    if (!this.lengthKnown && buf.length < this.chunkLength) {
      this.lengthKnown = true;
      this.lastChunkIndex = index;
      this.lastChunkLength = buf.length;
    }

    const isLastChunk = this.lengthKnown && index === this.lastChunkIndex;
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

    let evictIterations = 0;
    const maxEvictIterations = this.lruList.size + 1;
    while (this.currentBytes + buf.length > this.maxBytes && this.lruList.size > 0) {
      if (evictIterations >= maxEvictIterations) {
        this._evictChunk(this.lruList.values().next().value);
      } else {
        const evictIndex = this._findEvictableIndex(index);
        if (evictIndex === null) break;
        this._evictChunk(evictIndex);
      }
      evictIterations++;
    }

    this.chunks.set(index, buf);
    this.lruList.add(index);
    this.currentBytes += buf.length;

    if (this.closed) {
      queueMicrotask(() => {
        if (cb) cb(new Error("Storage is closed"));
      });
      return;
    }
    if (cb) {
      this.baseStore.put(index, buf, cb);
    } else {
      this.baseStore.put(index, buf);
    }
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
    let bestIndex = null;
    let bestDistance = -1;
    for (const idx of this.lruList) {
      if (idx === currentIndex) continue;
      const distance = Math.abs(idx - currentIndex);
      if (distance > bestDistance) {
        bestDistance = distance;
        bestIndex = idx;
      }
    }
    return bestIndex;
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
