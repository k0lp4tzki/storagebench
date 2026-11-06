import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output goes to ../cmd/controller/webdist so the Go build can embed it
// with `-tags embedui`. During development, `npm run dev` proxies API and
// WebSocket calls to a locally running controller on :8080.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../cmd/controller/webdist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/ws": {
        target: "ws://127.0.0.1:8080",
        ws: true,
      },
    },
  },
});
