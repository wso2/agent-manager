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

import { PLUGIN_API_VERSION } from "@agent-management-platform/types";
import { builtinModules } from "./builtinModules";

describe("builtinModules", () => {
  it("lists the shipped modules in Module manager order", () => {
    expect(builtinModules.map((m) => m.id)).toEqual([
      "acp-core",
      "ai-gateway",
      "ai-observability",
      "agent-identity",
      "ai-hub",
    ]);
  });

  it("targets the console's plugin API, so none is skipped at startup", () => {
    for (const module of builtinModules) {
      expect(module.apiVersion).toBe(PLUGIN_API_VERSION);
      for (const plugin of module.plugins) {
        expect(plugin.apiVersion).toBe(PLUGIN_API_VERSION);
      }
    }
  });

  it("uses plugin ids that are unique across modules", () => {
    const ids = builtinModules.flatMap((m) => m.plugins.map((p) => p.id));
    expect(new Set(ids).size).toBe(ids.length);
  });
});
