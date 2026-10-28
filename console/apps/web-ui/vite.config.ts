import fs from 'node:fs'
import path from 'node:path'
import { defineConfig, loadEnv, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import { workspaceSourceAliases } from '../../workspace-aliases.mjs'

// Dev-server stand-in for the production image's envsubst step (scripts/40-config-setup.sh):
// serves config.js from config.template.js, filled from .env.development with the process
// env on top (e.g. DISABLE_AUTH=true pnpm dev). Unset placeholders become '', as with envsubst.
function runtimeConfig(): Plugin {
  return {
    name: 'runtime-config',
    apply: 'serve',
    configureServer(server) {
      const { mode, envDir, publicDir, base } = server.config
      server.middlewares.use(`${base}config.js`, (_req, res) => {
        const env = loadEnv(mode, envDir, '')
        const template = fs.readFileSync(path.join(publicDir, 'config.template.js'), 'utf8')
        res.setHeader('Content-Type', 'text/javascript')
        const render = (_match: string, braced?: string, bare?: string) => env[braced ?? bare ?? ''] ?? ''
        res.end(template.replace(/\$(?:\{(\w+)\}|(\w+))/g, render))
      })
    },
  }
}

// URL path prefix the console is served under (e.g. "/console" behind a
// shared host). Baked into asset URLs at build time; the same BASE_PATH env reaches
// the runtime config (config.template.js -> basePath) for the router basename.
const basePath = (process.env.BASE_PATH ?? '/').trim().replace(/^\/+|\/+$/g, '')
const base = basePath ? `/${basePath}/` : '/'

// https://vite.dev/config/
export default defineConfig({
  base,
  plugins: [react(), runtimeConfig()],
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    // Resolve core-ui and every package it pulls in to source, so dev hot-reloads
    // across the whole workspace with no prior build step.
    alias: workspaceSourceAliases({ exclude: ['web-ui'] }),
  },
  server: {
    port: 3000,
    // Login only works on :3000 (Thunder redirect URIs), so fail instead of moving to 3001.
    strictPort: true,
  },
  build: {
    // The main app chunk sits at ~5 MB; PR builds bundle the branch merged with
    // main, which pushes it just past a 5000 kB limit. Keep headroom so the Vite
    // chunk-size warning (treated as a build failure in CI) doesn't trip.
    chunkSizeWarningLimit: 6000,
  },
})
