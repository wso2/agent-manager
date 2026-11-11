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

import type { EnvironmentVariable } from "@agent-management-platform/types";

// Its value lives in the secret store and never reaches the console, so the key/value
// fields are locked by default (EnvVariableEditor's isExistingSecret) until unlocked via
// the Edit action or by unmarking "Mark as Secret" — either way, secretRef is preserved
// unless a new value is typed (see toSubmittableEnv below).
export function isStoredSecret(item: EnvironmentVariable): boolean {
  return !!(item.isSensitive && item.secretRef);
}

/**
 * Sorts system-injected entries (isSystem=true) below user-managed ones,
 * preserving relative order within each group.
 */
export function sortSystemLast<T extends { isSystem?: boolean }>(items: T[]): T[] {
  return [...items].sort((a, b) => (a.isSystem ? 1 : 0) - (b.isSystem ? 1 : 0));
}

/**
 * Drops system-injected entries: they're platform-managed and re-applied by
 * the backend independently of any save/promote payload.
 */
export function excludeSystemVars<T extends { key: string; isSystem?: boolean }>(
  items: T[],
): T[] {
  return items.filter((item) => item.key && !item.isSystem);
}

/**
 * Builds the env payload for save/promote requests. Preserves secretRef for secrets the
 * user did not edit — checked on secretRef alone (not isSensitive) because unmarking
 * "Mark as Secret" is one way to unlock the key field for renaming (see isStoredSecret /
 * EnvVariableEditor's Edit action) — the row still backs onto the same stored secret and
 * must keep forwarding its ref, or the rename silently orphans it.
 */
export function toSubmittableEnv(items: EnvironmentVariable[]): EnvironmentVariable[] {
  return excludeSystemVars(items).map(({ key, value, isSensitive, secretRef }) => {
    if (secretRef && !value) {
      // The cast is intentional: EnvironmentVariable.value is typed as required, but the
      // backend's contract for this shape is "value XOR secretRef" — omitting value here
      // (rather than sending an empty string) is what tells it to keep the stored secret
      // rather than overwrite it. Narrowing the shared type isn't safe to do just for this
      // call site since EnvironmentVariable also models GET responses, where value is
      // always present.
      return { key, isSensitive: true, secretRef } as EnvironmentVariable;
    }
    return { key, value, isSensitive };
  });
}
