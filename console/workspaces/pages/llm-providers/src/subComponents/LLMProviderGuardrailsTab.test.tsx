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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import type { LLMProviderResponse } from "@agent-management-platform/types";

const catalog = vi.hoisted(() => ({
  state: { data: undefined as unknown, error: null as unknown },
}));

vi.mock("@agent-management-platform/api-client", async () => ({
  normalizePolicyVersionToMajor: (
    await import("../../../../libs/api-client/src/utils/policyVersion")
  ).normalizePolicyVersionToMajor,
  useLLMPoliciesCatalog: () => ({
    data: catalog.state.data,
    error: catalog.state.error,
    isLoading: false,
  }),
  filterGuardrailPolicies: (items: unknown[]) => items,
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: true }),
}));
vi.mock("../hooks/useOpenApiSpec", () => ({
  useOpenApiSpec: () => ({ text: "", isLoading: false, error: null }),
}));

import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { LLMProviderGuardrailsTab } from "./LLMProviderGuardrailsTab";

const provider = {
  uuid: "prov-1",
  openapi: "",
  policies: [
    {
      name: "legacy-guard",
      version: "v1.0.0",
      paths: [{ path: "/*", methods: ["*"], params: { mode: "strict" } }],
    },
  ],
} as unknown as LLMProviderResponse;

const renderTab = (onUpdate = vi.fn()) =>
  render(
    <MemoryRouter>
      <ConfirmationDialogProvider>
        <LLMProviderGuardrailsTab
          providerData={provider}
          onUpdate={onUpdate}
          isUpdating={false}
        />
      </ConfirmationDialogProvider>
    </MemoryRouter>,
  );

const loadedCatalog = (data: unknown[], availabilityKnown = true) => ({
  count: data.length,
  data,
  hubAvailable: true,
  availabilityScope: "deployed",
  availabilityKnown,
});

describe("LLMProviderGuardrailsTab while availability is unknown", () => {
  it("does not flag an applied guardrail and opens it read-only, with the info note, when no entry has a schema", () => {
    catalog.state = {
      data: loadedCatalog(
        [{ name: "legacy-guard", version: "v1.0.0", status: "not_installed" }],
        false,
      ),
      error: null,
    };
    renderTab();
    expect(screen.queryByText("Not available on this gateway")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText(/legacy-guard/));
    expect(screen.getByRole("presentation").textContent).toContain('"mode": "strict"');
    expect(screen.getByText(/hasn't reported its policies yet/)).toBeInTheDocument();
    expect(screen.queryByText(/is not available on this gateway/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^save$/i })).not.toBeInTheDocument();
  });

  it("edits an applied guardrail with the schema of a not-installed catalog entry", () => {
    catalog.state = {
      data: loadedCatalog(
        [
          {
            name: "legacy-guard",
            version: "v1.2.0",
            status: "not_installed",
            displayName: "legacy-guard",
            provider: "hub",
            source: "hub",
            categories: [],
            description: "",
            isLatest: true,
            parameters: { type: "object", properties: { mode: { type: "string" } } },
          },
        ],
        false,
      ),
      error: null,
    };
    renderTab();
    expect(screen.queryByText("Not available on this gateway")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText(/legacy-guard/));
    expect(screen.getByDisplayValue("strict")).toBeInTheDocument();
    expect(screen.queryByText(/is not available on this gateway/)).not.toBeInTheDocument();
  });
});

describe("LLMProviderGuardrailsTab unavailable guardrails", () => {
  it("flags an applied guardrail missing from the catalog, opens it read-only and keeps Remove", () => {
    catalog.state = { data: loadedCatalog([]), error: null };
    const onUpdate = vi.fn().mockResolvedValue(provider);
    renderTab(onUpdate);

    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();

    fireEvent.click(screen.getByText(/legacy-guard/));
    expect(screen.getByRole("presentation").textContent).toContain(
      '"mode": "strict"',
    );
    expect(screen.queryByRole("button", { name: /^save$/i })).not.toBeInTheDocument();
  });

  it("flags an applied guardrail when only a different major is installed", () => {
    catalog.state = {
      data: loadedCatalog([
        { name: "legacy-guard", version: "v2.0.0", status: "installed", parameters: {} },
      ]),
      error: null,
    };
    renderTab();
    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();
    fireEvent.click(screen.getByText(/legacy-guard/));
    expect(screen.queryByRole("button", { name: /^save$/i })).not.toBeInTheDocument();
  });

  it("does not flag an applied guardrail when a same-major version is installed", () => {
    catalog.state = {
      data: loadedCatalog([
        { name: "legacy-guard", version: "v1.2.0", status: "installed", parameters: {} },
      ]),
      error: null,
    };
    renderTab();
    expect(screen.getByText(/legacy-guard/)).toBeInTheDocument();
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
  });

  it("flags an applied guardrail when only a not-installed version is listed", () => {
    catalog.state = {
      data: loadedCatalog([
        { name: "legacy-guard", version: "v1.0.0", status: "not_installed" },
      ]),
      error: null,
    };
    renderTab();
    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();
  });

  it("lets an unavailable guardrail be removed", () => {
    catalog.state = { data: loadedCatalog([]), error: null };
    renderTab();
    const chip = screen.getByText(/legacy-guard/).closest(".MuiChip-root")!;
    fireEvent.click(chip.querySelector(".MuiChip-deleteIcon")!);
    expect(screen.queryByText(/legacy-guard/)).not.toBeInTheDocument();
  });

  it("shows no warning while the catalog is still loading", () => {
    catalog.state = { data: undefined, error: null };
    renderTab();
    expect(screen.getByText(/legacy-guard/)).toBeInTheDocument();
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
  });

  it("shows no warning when the catalog failed to load", () => {
    catalog.state = { data: undefined, error: new Error("boom") };
    renderTab();
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
  });
});

describe("LLMProviderGuardrailsTab editing an older saved version", () => {
  it("opens the installed v1.2.0 form and saves the params under the saved v1.0.0", async () => {
    catalog.state = {
      data: loadedCatalog([
        {
          name: "legacy-guard",
          version: "v1.2.0",
          displayName: "Legacy Guard",
          status: "installed",
          source: "hub",
          provider: "hub",
          categories: [],
          isLatest: true,
          description: "",
          parameters: {
            type: "object",
            properties: { mode: { type: "string" } },
          },
        },
      ]),
      error: null,
    };
    const onUpdate = vi.fn().mockResolvedValue(provider);
    renderTab(onUpdate);

    expect(screen.getByText(/Legacy Guard/)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/Legacy Guard/));
    // The editor is prefilled, not the picker list.
    const field = screen.getByDisplayValue("strict");
    fireEvent.change(field, { target: { value: "lenient" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    const [{ policies }] = onUpdate.mock.calls[0];
    expect(policies[0]).toMatchObject({
      name: "legacy-guard",
      version: "v1.0.0",
      paths: [{ params: { mode: "lenient" } }],
    });
  });
});
