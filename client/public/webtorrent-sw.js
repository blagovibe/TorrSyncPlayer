const portTimeoutDuration = 15000;

const listener = (event) => {
  // Only handle requests from our own origin
  if (event.clientId) {
    const client = clients.get(event.clientId).catch(() => null);
    if (!client) return null;
  }

  const { url } = event.request;
  if (event.request.method !== "GET") return null;
  if (!url.includes(self.registration.scope + "webtorrent/")) return null;
  if (url.includes(self.registration.scope + "webtorrent/keepalive/")) return new Response();
  if (url.includes(self.registration.scope + "webtorrent/cancel/")) {
    return new Response(
      new ReadableStream({
        cancel() {
          // Per-request cancellation is handled via the port below
        },
      }),
    );
  }
  return serve(event);
};

self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("fetch", (event) => {
  const res = listener(event);
  if (res) event.respondWith(res);
});

self.addEventListener("activate", () => {
  self.clients.claim();
});

async function serve({ request }) {
  const { url, method, headers, destination } = request;
  const clientlist = await clients.matchAll({ type: "window", includeUncontrolled: true });

  // Filter to only include clients from our origin
  const ourClients = [];
  for (const client of clientlist) {
    if (client.url.startsWith(self.registration.scope)) {
      ourClients.push(client);
    }
  }

  if (ourClients.length === 0) {
    return new Response("No available client", { status: 503 });
  }

  const [data, port] = await new Promise((resolve) => {
    for (const client of ourClients) {
      const messageChannel = new MessageChannel();
      const { port1, port2 } = messageChannel;
      port1.onmessage = ({ data: response }) => {
        resolve([response, port1]);
      };
      client.postMessage(
        {
          url,
          method,
          headers: Object.fromEntries(headers.entries()),
          scope: self.registration.scope,
          destination,
          type: "webtorrent",
        },
        [port2],
      );
    }
  });

  let timeOut = null;
  const cleanup = () => {
    port.postMessage(false);
    clearTimeout(timeOut);
    port.onmessage = null;
  };

  if (data.body !== "STREAM") {
    cleanup();
    return new Response(data.body, data);
  }

  return new Response(
    new ReadableStream({
      pull(controller) {
        return new Promise((resolve) => {
          port.onmessage = ({ data: chunk }) => {
            if (chunk) {
              controller.enqueue(chunk);
            } else {
              cleanup();
              controller.close();
            }
            resolve();
          };
          clearTimeout(timeOut);
          // Add a longer timeout even for document destinations
          timeOut = setTimeout(() => {
            cleanup();
            resolve();
          }, 30_000);
          port.postMessage(true);
        });
      },
      cancel() {
        cleanup();
      },
    }),
    data,
  );
}
