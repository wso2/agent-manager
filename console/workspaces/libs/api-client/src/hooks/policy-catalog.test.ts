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

import { describe, expect, it, vi } from "vitest";
import type { PolicyCatalogResponse } from "@agent-management-platform/types";
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t" }),
}));

import { toGuardrailCatalog } from "./guardrails";
import { toMCPCatalog } from "./mcp-policies";

const response = (
  overrides: Partial<PolicyCatalogResponse> = {},
): PolicyCatalogResponse => ({
  count: 2,
  hubAvailable: true,
  availabilityScope: "deployed",
  list: [
    {
      name: "pii-masking",
      version: "v1.0.2",
      displayName: "PII Masking",
      description: "Masks PII",
      source: "hub",
      status: "not_installed",
      missingOn: [{ id: "gw-1", name: "Gateway One" }],
      categories: ["Guardrails"],
    },
    {
      name: "mcp-rewrite",
      version: "v1.0.2",
      source: "custom",
      status: "installed",
      categories: [],
      parameters: {},
    },
  ],
  ...overrides,
});

describe("toGuardrailCatalog", () => {
  it("maps a not-installed item without parameters", () => {
    const { data } = toGuardrailCatalog(response());
    expect(data[0]).toMatchObject({
      name: "pii-masking",
      version: "v1.0.2",
      displayName: "PII Masking",
      provider: "hub",
      status: "not_installed",
      source: "hub",
      missingOn: [{ id: "gw-1", name: "Gateway One" }],
      categories: ["Guardrails"],
      isLatest: true,
    });
    expect(data[0].parameters).toBeUndefined();
  });

  it("keeps empty parameters defined for installed items", () => {
    const { data } = toGuardrailCatalog(response());
    expect(data[1].parameters).toEqual({});
    expect(data[1].provider).toBe("custom");
    expect(data[1].displayName).toBe("mcp-rewrite");
    expect(data[1].description).toBe("");
  });

  it("passes hubAvailable, availabilityScope and count through", () => {
    const out = toGuardrailCatalog(
      response({ hubAvailable: false, availabilityScope: "org_managed" }),
    );
    expect(out.hubAvailable).toBe(false);
    expect(out.availabilityScope).toBe("org_managed");
    expect(out.count).toBe(2);
  });
});

describe("toMCPCatalog", () => {
  it("derives savedVersion as the major version", () => {
    const { data } = toMCPCatalog(response());
    expect(data[0].version).toBe("v1.0.2");
    expect(data[0].savedVersion).toBe("v1");
    expect(data[1].savedVersion).toBe("v1");
  });

  it("passes catalog metadata through", () => {
    const out = toMCPCatalog(
      response({ hubAvailable: false, availabilityScope: "org_managed" }),
    );
    expect(out.hubAvailable).toBe(false);
    expect(out.availabilityScope).toBe("org_managed");
  });
});
