import animate from "tailwindcss-animate";

// "Organic" design-system tokens backed by the CSS variables in src/index.css.
// Light/dark theming happens entirely in the variables (the .dark block flips
// each tonal ramp), so these mappings stay theme-agnostic.
const v = (name) => `var(--color-${name})`;
const ramp = (name) =>
  Object.fromEntries(
    [100, 200, 300, 400, 500, 600, 700, 800, 900].map((step) => [step, v(`${name}-${step}`)])
  );

/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        sans: ["Figtree", "system-ui", "sans-serif"],
        display: ["Caprasimo", "system-ui", "sans-serif"],
      },
      colors: {
        bg: v("bg"),
        surface: v("surface"),
        ink: v("text"),
        divider: v("divider"),
        accent: { DEFAULT: v("accent"), ...ramp("accent") },
        "accent-2": { DEFAULT: v("accent-2"), ...ramp("accent-2") },
        // Primary-action fill. Split from the `accent` ramp because each theme
        // picks its own value to clear AA against the label it carries.
        "accent-solid": { DEFAULT: v("accent-solid"), hover: v("accent-solid-hover") },
        neutral: ramp("neutral"),
      },
      borderRadius: {
        // Cards in the Organic system soften to ~radius-lg * 1.15.
        card: "32px",
      },
      // Elevation — soft ink-tinted shadows derived from neutral-900.
      boxShadow: {
        sm: "0 1px 2px rgba(46, 43, 37, 0.14)",
        md: "0 3px 10px rgba(46, 43, 37, 0.16)",
        lg: "0 12px 32px rgba(46, 43, 37, 0.22)",
      },
    },
  },
  // Provides the animate-in / fade-in / slide-in-from-* utilities the views
  // use for entrance animations.
  plugins: [animate],
};
