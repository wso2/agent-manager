/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

const LEGACY_ORG_SEGMENT = 'org';
const ORG_SEGMENT = 'organizations';
const LEGACY_PROJECT_SEGMENT = 'project';
const PROJECT_SEGMENT = 'projects';

/**
 * Where to send a link that uses the old singular route segments
 * (`/org/:orgId/project/:projectId/...`) now that routes are
 * `/organizations/:orgId/projects/:projectId/...`. Any trailing sub-path is
 * kept. Undefined when the path needs no redirect.
 */
export function legacyRoutePath(pathname: string): string | undefined {
  const segments = pathname.split('/');
  // segments[0] is the empty string before the leading slash.
  const [, root, orgId, scope, projectId] = segments;
  if (root !== LEGACY_ORG_SEGMENT && root !== ORG_SEGMENT) {
    return undefined;
  }
  if (!orgId) {
    return undefined;
  }
  let changed = false;
  if (root === LEGACY_ORG_SEGMENT) {
    segments[1] = ORG_SEGMENT;
    changed = true;
  }
  if (scope === LEGACY_PROJECT_SEGMENT && projectId) {
    segments[3] = PROJECT_SEGMENT;
    changed = true;
  }
  return changed ? segments.join('/') : undefined;
}
