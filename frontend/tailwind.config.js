import animate from "tailwindcss-animate";

// Material 3 color roles backed by the CSS variables in src/index.css.
// Channel triplets + <alpha-value> keep opacity modifiers (bg-primary/30) working.
const token = (name) => `rgb(var(--md-${name}) / <alpha-value>)`;

/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        sans: ["Roboto", "system-ui", "sans-serif"],
      },
      colors: {
        primary: token("primary"),
        "on-primary": token("on-primary"),
        "primary-container": token("primary-container"),
        "on-primary-container": token("on-primary-container"),
        "secondary-container": token("secondary-container"),
        "inverse-primary": token("inverse-primary"),
        surface: token("surface"),
        "on-surface": token("on-surface"),
        "on-surface-variant": token("on-surface-variant"),
        "surface-container": token("surface-container"),
        "surface-container-high": token("surface-container-high"),
        "surface-container-lowest": token("surface-container-lowest"),
        outline: token("outline"),
        "outline-variant": token("outline-variant"),
        error: token("error"),
        "on-error": token("on-error"),
        hero: token("hero"),
        "hero-accent": token("hero-accent"),
      },
      // Material 3 type scale (sizes/leading/tracking; weight stays per-usage).
      fontSize: {
        "display-lg": ["3.5625rem", { lineHeight: "4rem", letterSpacing: "-0.016em" }],
        "display-md": ["2.8125rem", { lineHeight: "3.25rem" }],
        "display-sm": ["2.25rem", { lineHeight: "2.75rem" }],
        "headline-lg": ["2rem", { lineHeight: "2.5rem" }],
        "headline-md": ["1.75rem", { lineHeight: "2.25rem" }],
        "headline-sm": ["1.5rem", { lineHeight: "2rem" }],
        "title-lg": ["1.375rem", { lineHeight: "1.75rem" }],
        "title-md": ["1rem", { lineHeight: "1.5rem", letterSpacing: "0.009em" }],
        "body-lg": ["1rem", { lineHeight: "1.5rem", letterSpacing: "0.031em" }],
        "body-md": ["0.875rem", { lineHeight: "1.25rem", letterSpacing: "0.016em" }],
        "label-lg": ["0.875rem", { lineHeight: "1.25rem", letterSpacing: "0.007em" }],
        "label-md": ["0.75rem", { lineHeight: "1rem", letterSpacing: "0.042em" }],
      },
    },
  },
  // Provides the animate-in / fade-in / slide-in-from-* utilities the
  // migrated NutriMind views use for entrance animations.
  plugins: [animate],
};
