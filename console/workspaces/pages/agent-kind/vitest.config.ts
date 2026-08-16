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
        // oxygen-ui's dist imports prismjs subpaths without file extensions,
        // which Node's native ESM loader rejects; route them through Vite's
        // resolver instead of externalizing them.
        inline: ['@wso2/oxygen-ui', '@mui/x-data-grid', 'prismjs'],
      },
    },
  },
});
