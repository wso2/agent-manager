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

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  catalog: [] as unknown[],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useLLMPoliciesCatalog: () => ({ data: { data: hoisted.catalog } }),
}));

// The real drawer fetches the catalog and policy definitions itself; this stub
// shows what it was opened for and submits a fixed choice.
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  PolicySelectorDrawer: (props: {
    open: boolean;
    title: string;
    editPolicyKey?: string;
    existingSettings?: Record<string, unknown>;
    onSubmit: (g: { name: string; version: string }, v: Record<string, unknown>) => void;
    onClose: () => void;
  }) =>
    props.open ? (
      <div role="dialog" aria-label={props.title}>
        <div>{`editing ${props.editPolicyKey ?? "none"} ${JSON.stringify(props.existingSettings ?? null)}`}</div>
        <button onClick={() => props.onSubmit({ name: "pii-masking", version: "v1.0.0" }, { level: "high" })}>
          submit pii
        </button>
        <button onClick={props.onClose}>close drawer</button>
      </div>
    ) : null,
}));

import type { LLMPolicy, LLMProviderResponse } from "@agent-management-platform/types";
import { makeProvider, makeSpec, renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderGuardrailsTab } from "./LLMProviderGuardrailsTab";

const onUpdate = vi.fn();

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { access = {} as Record<string, boolean>, error = null as Error | null, isLoading = false } = {},
) =>
  renderWithProviders(
    <LLMProviderGuardrailsTab
      providerData={providerData}
      isLoading={isLoading}
      error={error}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
    { access },
  );

const globalPolicy: LLMPolicy = {
  name: "pii-masking",
  version: "v1.0.0",
  paths: [{ path: "/*", methods: ["*"], params: { level: "low" } }],
};
const resourcePolicy: LLMPolicy = {
  name: "word-count",
  version: "1.2.0",
  paths: [{ path: "/chat", methods: ["POST"], params: {} }],
};

const spec = makeSpec(["POST /chat", "GET /models"]);
const save = () => screen.getByRole("button", { name: "Save" });
const deleteChip = (label: string) => {
  const chip = screen.getByText(label).closest(".MuiChip-root") as HTMLElement;
  fireEvent.click(chip.querySelector(".MuiChip-deleteIcon") as Element);
};

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(makeProvider());
  hoisted.catalog = [
    { name: "pii-masking", version: "v1.0.0", displayName: "PII Masking" },
    { name: "word-count", version: "1.2.0", displayName: "Word Count" },
  ];
});

describe("LLMProviderGuardrailsTab", () => {
  it("shows global and per-resource guardrails by their catalog names", () => {
    renderTab(makeProvider({ policies: [globalPolicy, resourcePolicy], openapi: spec }));
    expect(screen.getByText("PII Masking (v1.0.0)")).toBeInTheDocument();
    // Resource guardrails sit in a collapsed accordion until expanded.
    fireEvent.click(screen.getByText("/chat"));
    expect(screen.getByText("Word Count (v1.2.0)")).toBeInTheDocument();
    fireEvent.click(screen.getByText("/models"));
    expect(screen.getByText("No guardrails added yet.")).toBeInTheDocument();
    expect(save()).toBeDisabled();
  });

  it("adds a global guardrail and saves it", async () => {
    renderTab(makeProvider({ openapi: spec }));
    fireEvent.click(screen.getAllByRole("button", { name: "Add guardrail" })[0]);
    expect(screen.getByRole("dialog", { name: "Guardrails" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "submit pii" }));

    expect(screen.getByText("PII Masking (v1.0.0)")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(save());

    await screen.findByText("Guardrails saved successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      policies: [
        { name: "pii-masking", version: "v1.0.0", paths: [{ path: "/*", methods: ["*"], params: { level: "high" } }] },
      ],
    });
  });

  it("adds a resource guardrail to the existing policy of the same name", async () => {
    renderTab(makeProvider({ policies: [globalPolicy], openapi: spec }));
    fireEvent.click(screen.getByText("/models"));
    fireEvent.click(screen.getAllByRole("button", { name: "Add guardrail" })[1]);
    fireEvent.click(screen.getByRole("button", { name: "submit pii" }));
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].policies).toEqual([
      {
        name: "pii-masking",
        version: "v1.0.0",
        paths: [
          { path: "/*", methods: ["*"], params: { level: "low" } },
          { path: "/models", methods: ["GET"], params: { level: "high" } },
        ],
      },
    ]);
  });

  it("edits a guardrail's settings in place", async () => {
    renderTab(makeProvider({ policies: [globalPolicy] }));
    fireEvent.click(screen.getByText("PII Masking (v1.0.0)"));
    const dialog = screen.getByRole("dialog", { name: "Edit Guardrail" });
    expect(within(dialog).getByText('editing pii-masking@v1.0.0 {"level":"low"}')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "submit pii" }));
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].policies[0].paths).toEqual([
      { path: "/*", methods: ["*"], params: { level: "high" } },
    ]);
  });

  it("removes a guardrail, dropping the policy once it has no paths, and discard restores it", async () => {
    renderTab(makeProvider({ policies: [globalPolicy, resourcePolicy], openapi: spec }));
    deleteChip("PII Masking (v1.0.0)");
    expect(screen.queryByText("PII Masking (v1.0.0)")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.getByText("PII Masking (v1.0.0)")).toBeInTheDocument();

    deleteChip("PII Masking (v1.0.0)");
    fireEvent.click(save());
    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].policies).toEqual([resourcePolicy]);
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("x"));
    renderTab(makeProvider());
    fireEvent.click(screen.getByRole("button", { name: "Add guardrail" }));
    fireEvent.click(screen.getByRole("button", { name: "submit pii" }));
    fireEvent.click(save());
    expect(await screen.findByText("Failed to save guardrails.")).toBeInTheDocument();
  });

  it("filters resources by search", () => {
    renderTab(makeProvider({ openapi: spec }));
    fireEvent.change(screen.getByPlaceholderText("Search resources"), { target: { value: "model" } });
    expect(screen.queryByText("/chat")).not.toBeInTheDocument();
    expect(screen.getByText("/models")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("Search resources"), { target: { value: "nothing" } });
    expect(screen.getByText("No resources match your search")).toBeInTheDocument();
  });

  it("says when there are no resources to configure", () => {
    renderTab(makeProvider());
    expect(screen.getByText("No resources found")).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    renderTab(makeProvider({ policies: [globalPolicy] }), {
      access: { llm_provider_write: false },
    });
    expect(screen.getByText("PII Masking (v1.0.0)")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add guardrail" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("PII Masking (v1.0.0)"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows the provider load error", () => {
    renderTab(undefined, { error: new Error("provider missing") });
    expect(screen.getByText("provider missing")).toBeInTheDocument();
  });

  it("renders nothing without a provider and placeholders while loading", () => {
    const { container, rerenderWith } = renderTab(undefined);
    expect(container).toBeEmptyDOMElement();
    rerenderWith(
      <LLMProviderGuardrailsTab
        providerData={makeProvider()}
        isLoading
        onUpdate={onUpdate}
        isUpdating={false}
      />,
    );
    expect(screen.queryByText("Global Guardrails")).not.toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });
});
