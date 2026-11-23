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

import {
  filterGuardrailPolicies,
  toGuardrailCatalog,
  type GuardrailDefinition,
} from "./guardrails";
import { toMCPCatalog } from "./mcp-policies";

const response = (
  overrides: Partial<PolicyCatalogResponse> = {},
): PolicyCatalogResponse => ({
  count: 2,
  hubAvailable: true,
  availabilityScope: "deployed",
  availabilityKnown: true,
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

describe("availabilityKnown", () => {
  it("is passed through by both mappers", () => {
    for (const known of [false, true]) {
      const resp = response({ availabilityKnown: known });
      expect(toGuardrailCatalog(resp).availabilityKnown).toBe(known);
      expect(toMCPCatalog(resp).availabilityKnown).toBe(known);
    }
  });

  it("defaults to true when the service omits it", () => {
    const legacy = response() as Partial<PolicyCatalogResponse>;
    delete legacy.availabilityKnown;
    expect(toGuardrailCatalog(legacy as PolicyCatalogResponse).availabilityKnown).toBe(true);
    expect(toMCPCatalog(legacy as PolicyCatalogResponse).availabilityKnown).toBe(true);
  });
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

describe("filterGuardrailPolicies", () => {
  const def = (overrides: Partial<GuardrailDefinition>): GuardrailDefinition => ({
    name: "p",
    version: "v1.0.0",
    displayName: "P",
    description: "",
    provider: "hub",
    source: "hub",
    status: "installed",
    categories: [],
    isLatest: true,
    ...overrides,
  });

  it("hides a not-installed AI-only item", () => {
    const out = filterGuardrailPolicies([
      def({ name: "ai-only", status: "not_installed", categories: ["AI"] }),
    ]);
    expect(out).toEqual([]);
  });

  it("shows a not-installed Guardrails item", () => {
    const out = filterGuardrailPolicies([
      def({ name: "pii", status: "not_installed", categories: ["AI", "Guardrails"] }),
    ]);
    expect(out.map((p) => p.name)).toEqual(["pii"]);
  });

  it("keeps an installed AI-only item unless it is excluded", () => {
    const out = filterGuardrailPolicies([
      def({ name: "ai-only", categories: ["AI"] }),
      def({ name: "jwt-auth", categories: ["AI"] }),
    ]);
    expect(out.map((p) => p.name)).toEqual(["ai-only"]);
  });

  it("shows a not-installed custom item with no categories", () => {
    const out = filterGuardrailPolicies([
      def({ name: "my-custom", source: "custom", status: "not_installed", categories: [] }),
    ]);
    expect(out.map((p) => p.name)).toEqual(["my-custom"]);
  });

  it("shows a gateway-derived partial row the hub does not know", () => {
    const out = filterGuardrailPolicies([
      def({
        name: "gw-only",
        status: "not_installed",
        categories: [],
        missingOn: [{ id: "g2", name: "gw-b" }],
      }),
    ]);
    expect(out.map((p) => p.name)).toEqual(["gw-only"]);
  });

  it("still hides an excluded not-installed custom item", () => {
    const out = filterGuardrailPolicies([
      def({ name: "jwt-auth", source: "custom", status: "not_installed", categories: [] }),
    ]);
    expect(out).toEqual([]);
  });
});
