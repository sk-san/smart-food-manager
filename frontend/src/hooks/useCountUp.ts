import { useEffect, useRef, useState } from 'react';

/**
 * Counts a figure up to `value`, starting from whatever was last shown. The
 * first run therefore climbs from zero, and a later change climbs from the
 * previous total — so logging waste animates the difference rather than
 * replaying the whole number.
 *
 * Reduced-motion viewers are handed the final value immediately, which also
 * keeps the figure honest for anything reading the DOM rather than watching it.
 */
export const useCountUp = (value: number, durationMs = 1100) => {
  const [shown, setShown] = useState(0);
  /** What is on screen right now. `shown` itself is a frame behind in here. */
  const shownRef = useRef(0);
  const frameRef = useRef<number | null>(null);

  const paint = (next: number) => {
    shownRef.current = next;
    setShown(next);
  };

  useEffect(() => {
    const from = shownRef.current;
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reduceMotion || durationMs <= 0 || from === value) {
      paint(value);
      return;
    }

    let start: number | null = null;
    const step = (now: number) => {
      if (start === null) start = now;
      const progress = Math.min(1, (now - start) / durationMs);
      // Ease-out cubic: the figure arrives quickly and settles, rather than
      // crawling the last stretch the way a linear ramp does.
      const eased = 1 - Math.pow(1 - progress, 3);
      paint(from + (value - from) * eased);
      if (progress < 1) {
        frameRef.current = requestAnimationFrame(step);
        return;
      }
      // Land exactly on the target; the eased fraction only approaches it.
      paint(value);
      frameRef.current = null;
    };
    frameRef.current = requestAnimationFrame(step);

    return () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    };
  }, [value, durationMs]);

  return shown;
};
