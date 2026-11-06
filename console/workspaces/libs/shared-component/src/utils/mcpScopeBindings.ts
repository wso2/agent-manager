/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/** One tool of the selected endpoint and the scopes currently assigned to it. */
export type ToolScopeBindingRow = {
  tool: string;
  scopes: ReadonlyArray<{ action: string }>;
};

/** A scope as the server holds it: its action and the tools it covers, across every endpoint. */
export type ScopeCatalogEntry = { action: string; tools: readonly string[] };

/** One scope's replacement tool list, as sent by Save. */
export type ScopeToolUpdate = { action: string; tools: string[] };

/**
 * Diffs the tool-to-scope table of the selected endpoint against the scopes the server holds and
 * returns only the tool-list updates needed.
 *
 * A scope belongs to the whole proxy, and its tool list covers the tools of every endpoint, but the
 * table only shows the selected endpoint's tools. The server replaces a scope's whole tool list, so
 * the update must keep the tools this table does not control: a scope's tools that belong to
 * another endpoint (and not to this one) are carried over unchanged. Without that, saving on one
 * endpoint erased what had been assigned through another.
 *
 * A tool the table does not list and no other endpoint lists is dropped, as the server would refuse
 * it (it only accepts tools the proxy discovered). A scope left with no tools is a valid saved
 * state, not a reason to delete it.
 */
export function computeScopeToolUpdates(
  rows: readonly ToolScopeBindingRow[],
  catalogScopes: readonly ScopeCatalogEntry[],
  otherEndpointTools: Iterable<string> = [],
): ScopeToolUpdate[] {
  const controlledTools = new Set(rows.map((row) => row.tool));
  const otherTools = new Set(otherEndpointTools);

  const desired = new Map<string, Set<string>>(
    catalogScopes.map((scope) => [
      scope.action,
      new Set(scope.tools.filter((tool) => otherTools.has(tool) && !controlledTools.has(tool))),
    ]),
  );
  for (const row of rows) {
    for (const scope of row.scopes) {
      desired.get(scope.action)?.add(row.tool);
    }
  }

  const sameTools = (a: ReadonlySet<string>, b: ReadonlySet<string>) =>
    a.size === b.size && [...a].every((tool) => b.has(tool));

  const updates: ScopeToolUpdate[] = [];
  for (const scope of catalogScopes) {
    const desiredTools = desired.get(scope.action) ?? new Set<string>();
    if (!sameTools(desiredTools, new Set(scope.tools))) {
      updates.push({ action: scope.action, tools: [...desiredTools] });
    }
  }
  return updates;
}
