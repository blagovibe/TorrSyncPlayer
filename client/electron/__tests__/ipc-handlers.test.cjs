const { validateIpcSender } = require("../ipc-handlers.cjs");

function makeEvent(frameUrl) {
  return {
    senderFrame: { url: frameUrl },
    sender: { id: 1 },
  };
}

function makeStaticServer(url) {
  return url ? { url } : null;
}

describe("validateIpcSender", () => {
  const devUrl = "http://127.0.0.1:1420";

  it("accepts requests from the static server origin", () => {
    const staticServer = makeStaticServer("http://127.0.0.1:3000");
    const event = makeEvent("http://127.0.0.1:3000/index.html");
    expect(validateIpcSender(event, staticServer, devUrl)).toBe(true);
  });

  it("accepts requests from the dev server origin", () => {
    const event = makeEvent("http://127.0.0.1:1420/index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(true);
  });

  it("rejects requests with no frame URL", () => {
    const event = { senderFrame: null, sender: { id: 1 } };
    expect(validateIpcSender(event, null, devUrl)).toBe(false);
  });

  it("rejects file:// protocol", () => {
    const event = makeEvent("file:///index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(false);
  });

  it("rejects non-http/https protocols", () => {
    const event = makeEvent("chrome-extension://abc/index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(false);
  });

  it("rejects requests from non-loopback hostnames", () => {
    const event = makeEvent("http://192.168.1.1:1420/index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(false);
  });

  it("rejects requests from external hostnames", () => {
    const event = makeEvent("http://evil.com:1420/index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(false);
  });

  it("rejects requests with port mismatch", () => {
    const staticServer = makeStaticServer("http://127.0.0.1:3000");
    const event = makeEvent("http://127.0.0.1:9999/index.html");
    expect(validateIpcSender(event, staticServer, devUrl)).toBe(false);
  });

  it("accepts localhost when dev server URL uses 127.0.0.1 (loopback normalization)", () => {
    const event = makeEvent("http://localhost:1420/index.html");
    expect(validateIpcSender(event, null, devUrl)).toBe(true);
  });

  it("accepts localhost when dev server URL also uses localhost", () => {
    const event = makeEvent("http://localhost:1420/index.html");
    expect(validateIpcSender(event, null, "http://localhost:1420")).toBe(true);
  });

  it("accepts 127.0.0.1 when dev server URL uses localhost (loopback normalization)", () => {
    const event = makeEvent("http://127.0.0.1:1420/index.html");
    expect(validateIpcSender(event, null, "http://localhost:1420")).toBe(true);
  });
});
