import animate from "tailwindcss-animate";

/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["Roboto", "system-ui", "sans-serif"],
      },
    },
  },
  // Provides the animate-in / fade-in / slide-in-from-* utilities the
  // migrated NutriMind views use for entrance animations.
  plugins: [animate],
};
