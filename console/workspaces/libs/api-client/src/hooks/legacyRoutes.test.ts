/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { describe, expect, it } from "vitest";
import { generatePath } from "react-router-dom";
import { absoluteRouteMap, legacyRoutePath } from "@agent-management-platform/types";

describe("legacyRoutePath", () => {
  it.each([
    ["/org/acme", "/organizations/acme"],
    ["/org/acme/", "/organizations/acme/"],
    ["/org/acme/catalog", "/organizations/acme/catalog"],
    ["/org/acme/projects", "/organizations/acme/projects"],
    ["/org/acme/project/p1", "/organizations/acme/projects/p1"],
    [
      "/org/acme/project/p1/agents/a1/environment/dev/observability/traces",
      "/organizations/acme/projects/p1/agents/a1/environment/dev/observability/traces",
    ],
    // Half-migrated: new org segment, old project segment.
    ["/organizations/acme/project/p1/agents/a1", "/organizations/acme/projects/p1/agents/a1"],
  ])("redirects %s -> %s", (input, expected) => {
    expect(legacyRoutePath(input)).toBe(expected);
  });

  it.each([
    ["/organizations/acme"],
    ["/organizations/acme/projects/p1/agents/a1"],
    ["/org"],
    ["/org/"],
    ["/login"],
    ["/"],
    // A project page with no project id isn't a legacy project route.
    ["/organizations/acme/project"],
  ])("leaves %s alone", (input) => {
    expect(legacyRoutePath(input)).toBeUndefined();
  });

  it("lands on the current route map's paths", () => {
    const agentPath = generatePath(
      absoluteRouteMap.children.org.children.projects.children.agents.path,
      { orgId: "acme", projectId: "p1", agentId: "a1" },
    );
    expect(legacyRoutePath("/org/acme/project/p1/agents/a1")).toBe(agentPath);
  });
});
