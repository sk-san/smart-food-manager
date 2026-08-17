const CACHE_NAME = "nutri-static-v1";
const APP_SHELL = [
  "./",
  "./manifest.webmanifest",
  "./icons/icon-192.png",
  "./icons/icon-512.png",
  "./icons/maskable-192.png",
  "./icons/maskable-512.png",
];

self.addEventListener("install", (event) => {
  // Do not call skipWaiting: an update should not replace the worker while an
  // older app bundle is still open in another tab.
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(APP_SHELL)));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) =>
        Promise.all(
          names
            .filter((name) => name.startsWith("nutri-static-") && name !== CACHE_NAME)
            .map((name) => caches.delete(name)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);

  if (
    request.method !== "GET" ||
    url.origin !== self.location.origin ||
    isBackendRequest(url.pathname)
  ) {
    return;
  }

  if (request.mode === "navigate") {
    event.respondWith(networkFirstNavigation(request));
    return;
  }

  if (["style", "script", "image", "font"].includes(request.destination)) {
    event.respondWith(cacheFirstStatic(request));
  }
});

function isBackendRequest(pathname) {
  const scopePath = new URL(self.registration.scope).pathname.replace(/\/$/, "");
  const relativePath = scopePath && pathname.startsWith(scopePath)
    ? pathname.slice(scopePath.length) || "/"
    : pathname;

  return (
    relativePath === "/api" ||
    relativePath.startsWith("/api/") ||
    relativePath === "/healthz" ||
    relativePath.startsWith("/healthz/")
  );
}

async function networkFirstNavigation(request) {
  try {
    const response = await fetch(request);
    if (isCacheable(response)) {
      const cache = await caches.open(CACHE_NAME);
      await cache.put(new URL("./", self.registration.scope), response.clone());
    }
    return response;
  } catch {
    return (
      (await caches.match(request, { ignoreSearch: true })) ||
      (await caches.match(new URL("./", self.registration.scope), {
        ignoreSearch: true,
      })) ||
      Response.error()
    );
  }
}

async function cacheFirstStatic(request) {
  const cached = await caches.match(request);
  if (cached) {
    return cached;
  }

  const response = await fetch(request);
  if (isCacheable(response)) {
    const cache = await caches.open(CACHE_NAME);
    await cache.put(request, response.clone());
  }
  return response;
}

function isCacheable(response) {
  return response.ok && response.type === "basic";
}
