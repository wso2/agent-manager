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

// Single source of truth for "resolve every internal workspace package to its
// TypeScript source". The vite builds use it so imports hot-reload and get inlined
// into the bundle; the rollup-plugin-dts pass uses it so declarations are inlined
// the same way and no unpublished workspace reference leaks into the tarball.
//
// Deriving the list from the filesystem keeps the three consumers from drifting
// apart, which is what happened while each maintained its own hand-written copy.

import fs from 'node:fs'
import path from 'node:path'

const MONOREPO_ROOT = import.meta.dirname

// Mirrors the globs in pnpm-workspace.yaml. core-ui is listed on its own because it
// is a single package rather than a directory of them.
const PACKAGE_PARENTS = ['workspaces/libs', 'workspaces/pages']
const STANDALONE_PACKAGES = ['workspaces/core-ui', 'apps/web-ui']

function readPackageName(packageDir) {
  const manifest = path.join(packageDir, 'package.json')
  if (!fs.existsSync(manifest)) return null
  return JSON.parse(fs.readFileSync(manifest, 'utf8')).name ?? null
}

function discoverPackageDirs() {
  const fromParents = PACKAGE_PARENTS.flatMap((parent) =>
    fs
      .readdirSync(path.join(MONOREPO_ROOT, parent), { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => path.join(MONOREPO_ROOT, parent, entry.name))
  )
  return [...fromParents, ...STANDALONE_PACKAGES.map((p) => path.join(MONOREPO_ROOT, p))]
}

// A package without a src/ directory has no source to alias to — eslint-config is
// config-only, and aliasing it would point imports at a directory that isn't there.
function discoverSourcePackages() {
  return discoverPackageDirs()
    .map((dir) => ({ dir, name: readPackageName(dir) }))
    .filter(({ dir, name }) => name && fs.existsSync(path.join(dir, 'src')))
    .sort((a, b) => a.name.localeCompare(b.name))
}

/**
 * Vite `resolve.alias` entries mapping each workspace package to its source.
 *
 * @param {{ exclude?: string[] }} [options] package names to leave unaliased —
 *   a package should not alias itself.
 * @returns {{ find: string, replacement: string }[]}
 */
export function workspaceSourceAliases({ exclude = [] } = {}) {
  return discoverSourcePackages()
    .filter(({ name }) => !exclude.includes(name))
    .map(({ dir, name }) => ({ find: name, replacement: path.join(dir, 'src') }))
}

/**
 * The same mapping as TypeScript `compilerOptions.paths`, for the declaration pass.
 *
 * Subpath imports get two candidates because both spellings exist in the tree:
 * `@…/env-thunders/metadata` lives at `src/metadata`, while
 * `@…/types/src/api/builds` already spells out `src/`. TypeScript tries each in
 * order and uses the first that resolves.
 *
 * @param {string} fromDir directory the emitted paths should be relative to.
 * @returns {Record<string, string[]>}
 */
export function workspaceSourcePaths(fromDir) {
  const paths = {}
  for (const { dir, name } of discoverSourcePackages()) {
    const relative = path.relative(fromDir, dir).split(path.sep).join('/')
    paths[name] = [`${relative}/src`]
    paths[`${name}/*`] = [`${relative}/src/*`, `${relative}/*`]
  }
  return paths
}
