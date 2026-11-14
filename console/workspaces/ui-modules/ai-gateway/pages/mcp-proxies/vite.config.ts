/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 */

/// <reference types="node" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import dts from "vite-plugin-dts";
import { peerDependencies } from "./package.json";

export default defineConfig({
  build: {
    rollupOptions: {
      external: [...Object.keys(peerDependencies)],
    },
    cssCodeSplit: false,
    sourcemap: false,
    emptyOutDir: true,
  },
  css: {
    modules: {
      generateScopedName: "[local]_[hash:base64:5]",
      localsConvention: "camelCase",
    },
  },
  plugins: [
    react(),
    dts({
      include: ["src"],
    }),
  ],
});
