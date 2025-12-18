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
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react({
      babel: {
        plugins: [['babel-plugin-react-compiler']],
      },
    }),
  ],
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    alias: {
      // Add alias for better module resolution
      '@': path.resolve(__dirname, './src'),
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
        path.resolve(__dirname, '../../common/temp'),
        // Allow serving files from node_modules
        'node_modules',
      ],
    },
  },
  build: {
    chunkSizeWarningLimit: 5000, // Set reasonable limit
    rollupOptions: {
      output: {
        manualChunks: (id) => {
          // Split node_modules into separate chunks
          if (id.includes('node_modules')) {
            // Agent management platform workspace packages
            if (id.includes('@agent-management-platform')) {
              // Extract the package name
              const match = id.match(/@agent-management-platform\/([^/]+)/);
              if (match) {
                const packageName = match[1];
                // Group related packages
                if (['overview', 'build', 'deploy', 'test', 'traces'].includes(packageName)) {
                  return `page-${packageName}`;
                }
                if (['add-new-agent', 'add-new-project'].includes(packageName)) {
                  return `page-create-${packageName}`;
                }
                if (['shared-component', 'views', 'types'].includes(packageName)) {
                  return 'platform-shared';
                }
              }
              return 'vendor-amp-other';
            }
            if (id.includes('@wso2/oxygen-ui')) {
              return 'vendor-oxygen-ui';
            }
            // Other vendor libraries
            return 'vendor-other';
          }
        },
      },
    },
  },
})
