/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });
import type { MCPEndpointConfig } from "@agent-management-platform/types";

type Selection = {
  name: string;
  version: string;
  displayName?: string;
  settings?: Record<string, unknown>;
};
type SectionProps = {
  policies: Selection[];
  onAdd: (selection: Selection) => void;
  onEdit: (selection: Selection) => void;
  onRemove: (name: string, version: string) => void;
  onReorder: (next: Selection[]) => void;
  readOnly: boolean;
  filterPolicies: (items: { name: string }[]) => { name: string }[];
  getPolicyDefinitionVersion: (policy: { version: string; policyHubVersion?: string }) => string;
};

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  section: null as SectionProps | null,
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useMCPPoliciesCatalog: () => ({ data: { data: [] }, isLoading: false, error: null }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  PolicyListSection: (props: SectionProps) => {
    hoisted.section = props;
    return (
      <ul>
        {props.policies.map((policy) => (
          <li key={policy.name}>{policy.displayName ?? policy.name}</li>
        ))}
      </ul>
    );
  },
}));

import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";

const onUpdate = vi.fn();

const acl = { name: "mcp-acl-list", version: "v1", params: {} };
const rateLimit = {
  name: "rate-limit",
  version: "v1",
  displayName: "Rate limit",
  params: { rps: 5 },
};
const pii = { name: "pii-mask", version: "v2", displayName: "PII mask", params: {} };

const renderTab = () =>
  render(
    <MCPProxyPoliciesTab
      config={{ id: "primary", policies: [rateLimit, acl, pii] } as unknown as MCPEndpointConfig}
      selectedEndpointId="primary"
      orgName="acme"
      onUpdate={onUpdate}
      isUpdating={false}
    />,
  );

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.section = null;
  onUpdate.mockReset().mockResolvedValue({});
});

describe("MCPProxyPoliciesTab", () => {
  it("lists only the policies the other tabs don't manage", () => {
    renderTab();

    expect(screen.getByText("Rate limit")).toBeInTheDocument();
    expect(screen.getByText("PII mask")).toBeInTheDocument();
    expect(screen.queryByText("mcp-acl-list")).not.toBeInTheDocument();
    expect(hoisted.section!.readOnly).toBe(false);
  });

  it("hides managed policies from the catalog and prefers the hub version", () => {
    renderTab();

    expect(
      hoisted.section!.filterPolicies([
        { name: "mcp-auth" },
        { name: "mcp-rewrite" },
        { name: "rate-limit" },
      ]),
    ).toEqual([{ name: "rate-limit" }]);
    expect(
      hoisted.section!.getPolicyDefinitionVersion({ version: "v1", policyHubVersion: "1.2.0" }),
    ).toBe("1.2.0");
    expect(hoisted.section!.getPolicyDefinitionVersion({ version: "v1" })).toBe("v1");
  });

  it("appends a newly added policy", () => {
    renderTab();

    hoisted.section!.onAdd({ name: "cors", version: "v1", displayName: "CORS", settings: { a: 1 } });

    expect(onUpdate).toHaveBeenCalledWith({
      policies: [
        rateLimit,
        acl,
        pii,
        { name: "cors", version: "v1", displayName: "CORS", params: { a: 1 } },
      ],
    });
  });

  it("replaces an edited policy in place", () => {
    renderTab();

    hoisted.section!.onEdit({
      name: "rate-limit",
      version: "v1",
      displayName: "Rate limit",
      settings: { rps: 10 },
    });

    expect(onUpdate.mock.calls[0][0].policies[0]).toEqual({
      name: "rate-limit",
      version: "v1",
      displayName: "Rate limit",
      params: { rps: 10 },
    });
  });

  it("removes a policy", () => {
    renderTab();

    hoisted.section!.onRemove("pii-mask", "v2");

    expect(onUpdate).toHaveBeenCalledWith({ policies: [rateLimit, acl] });
  });

  it("reorders the visible policies around the managed ones", () => {
    renderTab();

    hoisted.section!.onReorder([
      { name: "pii-mask", version: "v2", displayName: "PII mask", settings: {} },
      { name: "rate-limit", version: "v1", displayName: "Rate limit", settings: { rps: 5 } },
    ]);

    expect(
      onUpdate.mock.calls[0][0].policies.map((p: { name: string }) => p.name),
    ).toEqual(["pii-mask", "mcp-acl-list", "rate-limit"]);
  });

  it("is read-only without write access", () => {
    hoisted.canWrite = false;
    renderTab();

    expect(hoisted.section!.readOnly).toBe(true);
    fireEvent.click(screen.getByText("Rate limit"));
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
