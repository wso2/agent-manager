/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import dts from 'vite-plugin-dts'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react({
      babel: {
        plugins: [['babel-plugin-react-compiler']],
      },
    }),
    dts({
      include: ['src'],
      outDir: 'dist',
      tsconfigPath: './tsconfig.app.json',
    }),
  ],
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    alias: {
      // Add alias for better module resolution
      '@': path.resolve(__dirname, './src'),

      // Workspace libraries - resolve to source for hot-reload without separate tsc watchers
      '@agent-management-platform/auth': path.resolve(__dirname, '../auth/src'),
      '@agent-management-platform/api-client': path.resolve(__dirname, '../api-client/src'),
      '@agent-management-platform/shared-component': path.resolve(__dirname, '../shared-component/src'),
      '@agent-management-platform/types': path.resolve(__dirname, '../types/src'),
      '@agent-management-platform/views': path.resolve(__dirname, '../views/src'),

      // Workspace pages - resolve to source for hot-reload
      '@agent-management-platform/add-new-agent': path.resolve(__dirname, '../../pages/add-new-agent/src'),
      '@agent-management-platform/add-new-project': path.resolve(__dirname, '../../pages/add-new-project/src'),
      '@agent-management-platform/build': path.resolve(__dirname, '../../pages/build/src'),
      '@agent-management-platform/deploy': path.resolve(__dirname, '../../pages/deploy/src'),
      '@agent-management-platform/overview': path.resolve(__dirname, '../../pages/overview/src'),
      '@agent-management-platform/configure-agent': path.resolve(__dirname, '../../pages/configure-agent/src'),
      '@agent-management-platform/test': path.resolve(__dirname, '../../pages/test/src'),
      '@agent-management-platform/traces': path.resolve(__dirname, '../../pages/traces/src'),
      '@agent-management-platform/logs': path.resolve(__dirname, '../../pages/logs/src'),
      '@agent-management-platform/metrics': path.resolve(__dirname, '../../pages/metrics/src'),
      '@agent-management-platform/eval': path.resolve(__dirname, '../../pages/eval/src'),
      '@agent-management-platform/llm-providers': path.resolve(__dirname, '../../pages/llm-providers/src'),
      '@agent-management-platform/gateways': path.resolve(__dirname, '../../pages/gateways/src'),
    },
  },
  server: {
    port: 3000,
    host: '0.0.0.0',
    watch: {
      // Reduce file watchers to avoid EMFILE errors
      // Note: workspace dist/ folders ARE watched for HMR
      ignored: [
        '**/node_modules/**',      // Exclude all node_modules (volume-mounted)
        '**/common/temp/**',       // Exclude Rush temp dir (volume-mounted)
        '**/.git/**',              // Exclude git
        '**/.rush/**',             // Exclude Rush cache
      ],
    },
    fs: {
      allow: [
        // Allow serving files from the project root
        '..',
        // Allow serving files from the common temp directory (for Rush.js monorepo)
        path.resolve(__dirname, '../../../common/temp'),
        // Allow serving files from node_modules
        'node_modules',
      ],
    },
  },
  build: {
    lib: {
      entry: path.resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: 'index',
    },
    rollupOptions: {
      external: [
        'react',
        'react/jsx-runtime',
        'react/compiler-runtime',
        'react-dom',
        'react-router-dom',
        '@mui/material',
        '@mui/icons-material',
        '@emotion/react',
        '@emotion/styled',
        '@wso2/oxygen-ui',
        '@wso2/oxygen-ui-icons-react',
        '@wso2/oxygen-ui-charts-react',
        '@tanstack/react-query',
        '@asgardeo/react',
      ],
    },
  },
})
