/// <reference types="vitest" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./setupTests.tsx",
    server: {
      deps: {
        // oxygen-ui's dist imports prismjs subpaths without file extensions
        // and @mui/x-data-grid imports raw .css, neither of which Node's
        // native ESM loader accepts; route them through Vite's resolver.
        inline: ["@wso2/oxygen-ui", "@mui/x-data-grid"],
      },
    },
  },
});
