import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { workspaceSourceAliases } from '../../workspace-aliases.mjs'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    // Resolve core-ui and every package it pulls in to source, so dev hot-reloads
    // across the whole workspace with no prior build step.
    alias: workspaceSourceAliases({ exclude: ['web-ui'] }),
  },
  server: {
    port: 3000,
  },
  build: {
    // The main app chunk sits at ~5 MB; PR builds bundle the branch merged with
    // main, which pushes it just past a 5000 kB limit. Keep headroom so the Vite
    // chunk-size warning (treated as a build failure in CI) doesn't trip.
    chunkSizeWarningLimit: 6000,
  },
})
