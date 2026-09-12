/*
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

// Bundles the TypeScript declarations for @wso2/am-core-ui into a single,
// self-contained dist/index.d.ts. The JS build (vite.config.ts) inlines the internal
// @agent-management-platform/* workspace packages via source aliases; this pass does
// the equivalent for types, resolving those packages through tsconfig.dts.json paths
// so no unpublished workspace references leak into the published tarball.
import dts from 'rollup-plugin-dts'
import { workspaceSourcePaths } from '../../workspace-aliases.mjs'

// Runtime peer dependencies — never inline their types, reference them as imports.
const external = [
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
]

// Style/asset imports carry no type information; stub them so the declaration
// bundler does not try to parse non-TypeScript files.
const ignoreStyles = {
  name: 'ignore-styles',
  resolveId(source) {
    if (/\.(css|scss|sass|less|svg|png|jpe?g|gif|woff2?)$/.test(source)) {
      return { id: source, external: true }
    }
    return null
  },
}

export default {
  input: 'src/index.ts',
  output: {
    file: 'dist/index.d.ts',
    format: 'es',
  },
  // Externalise peers plus any style/asset import that slips through.
  external: (id) =>
    external.includes(id) || /\.(css|scss|sass|less|svg|png|jpe?g|gif|woff2?)$/.test(id),
  plugins: [
    ignoreStyles,
    dts({
      tsconfig: './tsconfig.dts.json',
      // Shared with the vite builds so the declaration pass inlines exactly the
      // packages the JS bundle does — see ../../workspace-aliases.mjs.
      compilerOptions: {
        baseUrl: import.meta.dirname,
        paths: {
          '@/*': ['./src/*'],
          ...workspaceSourcePaths(import.meta.dirname),
        },
      },
      respectExternal: false,
    }),
  ],
}
