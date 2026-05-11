const portTimeoutDuration = 5000;
let cancellable = false;

const listener = (event) => {
  const { url } = event.request;
  if (!url.includes(self.registration.scope + "webtorrent/")) return null;
  if (url.includes(self.registration.scope + "webtorrent/keepalive/")) return new Response();
  if (url.includes(self.registration.scope + "webtorrent/cancel/")) {
    return new Response(
      new ReadableStream({
        cancel() {
          cancellable = true;
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

  const [data, port] = await new Promise((resolve) => {
    for (const client of clientlist) {
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
          if (!cancellable) {
            clearTimeout(timeOut);
            if (destination !== "document") {
              timeOut = setTimeout(() => {
                cleanup();
                resolve();
              }, portTimeoutDuration);
            }
          }
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
