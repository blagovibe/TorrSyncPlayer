const { BoundedChunkStore } = require("../bounded-chunk-store.cjs");

function put(store, index, buf) {
  return new Promise((resolve, reject) => {
    store.put(index, buf, (err) => (err ? reject(err) : resolve()));
  });
}

function get(store, index, opts) {
  return new Promise((resolve, reject) => {
    store.get(index, opts || null, (err, data) => (err ? reject(err) : resolve(data)));
  });
}

function destroy(store) {
  return new Promise((resolve, reject) => {
    store.destroy((err) => (err ? reject(err) : resolve()));
  });
}

describe("BoundedChunkStore", () => {
  it("stores and retrieves chunks", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 100 });
    const buf = Buffer.from("0123456789");
    await put(store, 0, buf);
    const data = await get(store, 0, null);
    expect(data).toEqual(buf);
    await destroy(store);
  });

  it("evicts chunks when maxBytes is exceeded", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 25 });
    await put(store, 0, Buffer.from("0123456789"));
    await put(store, 1, Buffer.from("abcdefghij"));
    await put(store, 2, Buffer.from("klmnopqrst"));
    expect(store.length).toBeLessThanOrEqual(25);
    await destroy(store);
  });

  it("rejects chunks with incorrect length", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 100 });
    await expect(put(store, 0, Buffer.from("short"))).rejects.toThrow();
    await destroy(store);
  });

  it("reports length as sum of stored chunk bytes", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 100 });
    await put(store, 0, Buffer.from("0123456789"));
    expect(store.length).toBe(10);
    await put(store, 1, Buffer.from("abcdefghij"));
    expect(store.length).toBe(20);
    await destroy(store);
  });

  it("rejects put after close", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 100 });
    await destroy(store);
    await expect(put(store, 0, Buffer.from("0123456789"))).rejects.toThrow();
  });

  it("rejects get after close", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 100 });
    await destroy(store);
    await expect(get(store, 0, null)).rejects.toThrow();
  });

  it("enforces maxBytes strictly", async () => {
    const store = new BoundedChunkStore(10, { maxBytes: 20 });
    await put(store, 0, Buffer.from("0123456789"));
    await put(store, 1, Buffer.from("abcdefghij"));
    expect(store.length).toBe(20);
    await put(store, 2, Buffer.from("klmnopqrst"));
    expect(store.length).toBeLessThanOrEqual(20);
    await destroy(store);
  });
});
