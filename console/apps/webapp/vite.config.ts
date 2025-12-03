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
    chunkSizeWarningLimit: 3000, // Increase limit to 3000 kB
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          // Keep MUI and Emotion together since MUI v7 depends on Emotion
          'vendor-mui': ['@mui/material', '@mui/icons-material', '@emotion/react', '@emotion/styled'],
        },
      },
    },
  },
})
