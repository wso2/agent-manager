import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    alias: [
      // More specific aliases MUST come before general ones (prefix matching order matters)
      { find: '@agent-management-platform/am-core-ui/dist/index.css', replacement: path.resolve(__dirname, '../../workspaces/core-ui/dist/index.css') },

      // Resolve core-ui and all its sub-packages to source for hot-reload
      { find: '@agent-management-platform/am-core-ui', replacement: path.resolve(__dirname, '../../workspaces/core-ui/src') },

      // Workspace libraries
      { find: '@agent-management-platform/auth', replacement: path.resolve(__dirname, '../../workspaces/libs/auth/src') },
      { find: '@agent-management-platform/api-client', replacement: path.resolve(__dirname, '../../workspaces/libs/api-client/src') },
      { find: '@agent-management-platform/shared-component', replacement: path.resolve(__dirname, '../../workspaces/libs/shared-component/src') },
      { find: '@agent-management-platform/types', replacement: path.resolve(__dirname, '../../workspaces/libs/types/src') },
      { find: '@agent-management-platform/views', replacement: path.resolve(__dirname, '../../workspaces/libs/views/src') },

      // Workspace pages
      { find: '@agent-management-platform/add-new-agent', replacement: path.resolve(__dirname, '../../workspaces/pages/add-new-agent/src') },
      { find: '@agent-management-platform/add-new-project', replacement: path.resolve(__dirname, '../../workspaces/pages/add-new-project/src') },
      { find: '@agent-management-platform/build', replacement: path.resolve(__dirname, '../../workspaces/pages/build/src') },
      { find: '@agent-management-platform/deploy', replacement: path.resolve(__dirname, '../../workspaces/pages/deploy/src') },
      { find: '@agent-management-platform/overview', replacement: path.resolve(__dirname, '../../workspaces/pages/overview/src') },
      { find: '@agent-management-platform/configure-agent', replacement: path.resolve(__dirname, '../../workspaces/pages/configure-agent/src') },
      { find: '@agent-management-platform/test', replacement: path.resolve(__dirname, '../../workspaces/pages/test/src') },
      { find: '@agent-management-platform/traces', replacement: path.resolve(__dirname, '../../workspaces/pages/traces/src') },
      { find: '@agent-management-platform/logs', replacement: path.resolve(__dirname, '../../workspaces/pages/logs/src') },
      { find: '@agent-management-platform/metrics', replacement: path.resolve(__dirname, '../../workspaces/pages/metrics/src') },
      { find: '@agent-management-platform/eval', replacement: path.resolve(__dirname, '../../workspaces/pages/eval/src') },
      { find: '@agent-management-platform/llm-providers', replacement: path.resolve(__dirname, '../../workspaces/pages/llm-providers/src') },
      { find: '@agent-management-platform/gateways', replacement: path.resolve(__dirname, '../../workspaces/pages/gateways/src') },
      { find: '@agent-management-platform/agent-security', replacement: path.resolve(__dirname, '../../workspaces/pages/agent-security/src') },
    ],
  },
  server: {
    port: 3000,
  },
  build: {
    chunkSizeWarningLimit: 5000,
  },
})
