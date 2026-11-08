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
import { environmentLabel, uniqueEnvironments } from "./environmentLabel";

describe("environmentLabel", () => {
  it("is the display name", () => {
    expect(environmentLabel({ id: "e1", name: "development", displayName: "Development" })).toBe("Development");
  });

  it("falls back to the name when the display name is blank or missing", () => {
    expect(environmentLabel({ id: "e1", name: "stage", displayName: "" })).toBe("stage");
    expect(environmentLabel({ id: "e1", name: "stage", displayName: "   " })).toBe("stage");
    expect(environmentLabel({ id: "e1", name: "stage" })).toBe("stage");
  });

  it("falls back to the ID, then to nothing", () => {
    expect(environmentLabel({ id: "e1", name: "", displayName: "" })).toBe("e1");
    expect(environmentLabel(undefined)).toBe("");
  });
});

describe("uniqueEnvironments", () => {
  it("lists an environment once, in order", () => {
    const list = [
      { id: "e1", name: "development" },
      { id: "e2", name: "stage" },
      { id: "e1", name: "development" },
    ];
    expect(uniqueEnvironments(list).map((e) => e.id)).toEqual(["e1", "e2"]);
  });

  it("tells environments apart by name when they have no ID", () => {
    expect(uniqueEnvironments([{ name: "a" }, { name: "b" }, { name: "a" }])).toHaveLength(2);
  });

  it("keeps the first of two with the same ID", () => {
    const list = [{ id: "e1", name: "first" }, { id: "e1", name: "second" }];
    expect(uniqueEnvironments(list)).toEqual([{ id: "e1", name: "first" }]);
  });
});
