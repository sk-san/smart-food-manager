import { useEffect, useState } from "react";

/** Below Tailwind's `md` breakpoint — the cut where the app swaps its chrome
 * from the desktop header to the bottom tab bar. */
export const MOBILE_QUERY = "(max-width: 767px)";

/** Live subscription to a media query. */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);

  useEffect(() => {
    const mql = window.matchMedia(query);
    // Re-read on subscribe: the query may have changed, or the viewport may
    // have moved between the initial render and this effect.
    setMatches(mql.matches);
    const onChange = (e: MediaQueryListEvent) => setMatches(e.matches);
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, [query]);

  return matches;
}

export const useIsMobile = () => useMediaQuery(MOBILE_QUERY);
