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

import { describe, expect, it } from "vitest";
import { computeScopeToolUpdates } from "./mcpScopeBindings";

const scope = (action: string, tools: string[]) => ({ action, tools });
const row = (tool: string, ...actions: string[]) => ({
  tool,
  scopes: actions.map((action) => ({ action })),
});

describe("computeScopeToolUpdates", () => {
  it("returns nothing when the table matches the server", () => {
    const rows = [row("search", "read"), row("write")];
    expect(computeScopeToolUpdates(rows, [scope("read", ["search"])])).toEqual([]);
  });

  it("adds and removes tools of the selected endpoint", () => {
    const rows = [row("search", "write"), row("write", "read")];
    const updates = computeScopeToolUpdates(rows, [
      scope("read", ["search"]),
      scope("write", []),
    ]);
    expect(updates).toEqual([
      { action: "read", tools: ["write"] },
      { action: "write", tools: ["search"] },
    ]);
  });

  it("keeps the tools another endpoint assigned when saving on this endpoint", () => {
    // Endpoint A has tool "a-tool", endpoint B has "b-tool". The scope covers both. Saving on B
    // with a change that touches only "b-tool" must not drop "a-tool".
    const rowsOfB = [row("b-tool", "read")];
    const catalog = [scope("read", ["a-tool"]), scope("admin", ["a-tool", "b-tool"])];
    const updates = computeScopeToolUpdates(rowsOfB, catalog, ["a-tool"]);
    expect(updates).toEqual([
      { action: "read", tools: ["a-tool", "b-tool"] },
      { action: "admin", tools: ["a-tool"] },
    ]);
  });

  it("does not touch a scope that only holds another endpoint's tools", () => {
    const updates = computeScopeToolUpdates([row("b-tool")], [scope("read", ["a-tool"])], ["a-tool"]);
    expect(updates).toEqual([]);
  });

  it("lets the table own a tool that both endpoints share", () => {
    // "shared" exists on both endpoints, so the table of this endpoint controls it.
    const updates = computeScopeToolUpdates([row("shared")], [scope("read", ["shared", "a-tool"])], [
      "shared",
      "a-tool",
    ]);
    expect(updates).toEqual([{ action: "read", tools: ["a-tool"] }]);
  });

  it("drops a tool that no endpoint lists, since the server would refuse it", () => {
    const updates = computeScopeToolUpdates([row("search", "read")], [scope("read", ["search", "gone"])], []);
    expect(updates).toEqual([{ action: "read", tools: ["search"] }]);
  });

  it("keeps everything of other endpoints when this endpoint lists no tools yet", () => {
    expect(computeScopeToolUpdates([], [scope("read", ["a-tool"])], ["a-tool"])).toEqual([]);
  });

  it("ignores a row's scope that is not in the catalog", () => {
    expect(computeScopeToolUpdates([row("search", "deleted")], [scope("read", [])])).toEqual([]);
  });
});
