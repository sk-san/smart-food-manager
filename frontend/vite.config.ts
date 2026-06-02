import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During dev, proxy API calls to the Go backend so the browser talks to a
// single origin and CORS stays simple.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "/healthz": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
});
