/* eslint-disable @typescript-eslint/triple-slash-reference */
/// <reference types="vitest" />
/// <reference types="node" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import dts from 'vite-plugin-dts';
import { peerDependencies } from './package.json';
import type { UserConfig } from 'vite';
import type { InlineConfig } from 'vitest';

interface VitestConfigExport extends UserConfig {
  test: InlineConfig;
}

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
      generateScopedName: '[local]_[hash:base64:5]',
      localsConvention: 'camelCase',
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './setupTests',
  },
  plugins: [
    react(),
    dts({
      exclude: ['**/*.stories.tsx', '**/*.test.tsx'],
      include: ['src'],
    }),
  ],
} as VitestConfigExport);
