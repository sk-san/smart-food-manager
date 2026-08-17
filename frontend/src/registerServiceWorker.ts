/** Register the production service worker after the first page load. */
export function registerServiceWorker(): void {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) {
    return;
  }

  window.addEventListener("load", () => {
    const baseUrl = import.meta.env.BASE_URL;

    navigator.serviceWorker
      .register(`${baseUrl}sw.js`, {
        scope: baseUrl,
        updateViaCache: "none",
      })
      .then((registration) => registration.update())
      .catch((error: unknown) => {
        // A failed registration should never prevent the online app from loading.
        console.warn("Nutri offline support could not be enabled.", error);
      });
  });
}
