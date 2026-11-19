/// <reference types="vitest" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./setupTests.tsx",
    // Oxygen's packages import their own CSS, which Node can't load: let Vite process them.
    server: {
      deps: {
        inline: ["@wso2/oxygen-ui", "@wso2/oxygen-ui-icons-react", "@mui/x-data-grid"],
      },
    },
  },
});
