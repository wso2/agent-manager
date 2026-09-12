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
import { workspaceSourceAliases } from '../../workspace-aliases.mjs'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react({
      babel: {
        plugins: [['babel-plugin-react-compiler']],
      },
    }),
  ],
  // NOTE: TypeScript declarations are produced by a separate rollup-plugin-dts pass
  // (see rollup.dts.config.mjs, wired into the `build` script). The JS bundle inlines
  // the internal @agent-management-platform/* workspace packages via the source
  // aliases below, so the declarations must be bundled the same way into a single
  // self-contained dist/index.d.ts. vite-plugin-dts / API Extractor cannot do this
  // here — it follows the aliased re-exports into raw .tsx sources and aborts.
  resolve: {
    dedupe: ['react', 'react-dom', 'react-router-dom'],
    alias: [
      // Every workspace package resolves to source, so the bundle inlines them and
      // watch mode picks up edits without separate tsc watchers.
      ...workspaceSourceAliases({ exclude: ['@agent-management-platform/am-core-ui'] }),

      { find: '@', replacement: path.resolve(__dirname, './src') },
    ],
  },
  build: {
    watch: process.env.VITE_WATCH ? {
      exclude: [
        '**/node_modules/**',
        '**/.git/**',
      ],
    } : undefined,
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
