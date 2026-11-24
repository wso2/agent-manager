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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type {
  GuardrailDefinition,
  GuardrailsCatalogResponse,
} from "@agent-management-platform/api-client";
import type { ConfirmationEvent } from "../ConfirmationDialog/ConfirmationDialogProvider";
import { PolicySelectorDrawer, type PolicySelectorDrawerProps } from "./PolicySelectorDrawer";

const useLLMPoliciesCatalog = vi.fn();
const useGuardrailPolicyDefinition = vi.fn();
const filterGuardrailPolicies = vi.fn((policies: GuardrailDefinition[]) =>
  policies.filter((p) => p.name !== "excluded"),
);
vi.mock("@agent-management-platform/api-client", () => ({
  useLLMPoliciesCatalog: (...args: unknown[]) => useLLMPoliciesCatalog(...args),
  useGuardrailPolicyDefinition: (...args: unknown[]) => useGuardrailPolicyDefinition(...args),
  filterGuardrailPolicies: (policies: GuardrailDefinition[]) => filterGuardrailPolicies(policies),
}));

// Capture the unsaved-changes dialog instead of rendering it.
const confirmations: ConfirmationEvent[] = [];
vi.mock("../ConfirmationDialog/ConfirmationDialogProvider", () => ({
  useConfirmationDialog: () => ({
    addConfirmation: (event: ConfirmationEvent) => confirmations.push(event),
  }),
}));

const inlinePolicy: GuardrailDefinition = {
  name: "pii-masking",
  displayName: "PII Masking",
  version: "1.0.0",
  description: "Masks personal data",
  parameters: {
    type: "object",
    required: ["pattern"],
    properties: { pattern: { type: "string" } },
  },
} as GuardrailDefinition;

const hubPolicy: GuardrailDefinition = {
  name: "word-count",
  version: "2.1.0",
  description: "x".repeat(250),
} as GuardrailDefinition;

const catalog = (data: GuardrailDefinition[]): GuardrailsCatalogResponse =>
  ({ count: data.length, data }) as GuardrailsCatalogResponse;

function renderDrawer(props: Partial<PolicySelectorDrawerProps> = {}) {
  const onClose = vi.fn();
  const onSubmit = vi.fn();
  const utils = render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={["/org/acme"]}>
        <Routes>
          <Route
            path="/org/:orgId"
            element={
              <PolicySelectorDrawer
                open
                onClose={onClose}
                onSubmit={onSubmit}
                catalogData={catalog([inlinePolicy, hubPolicy])}
                {...props}
              />
            }
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );
  return { ...utils, onClose, onSubmit };
}

const inputByLabel = (label: string) =>
  (screen.getByText(label).parentElement as HTMLElement).querySelector("input") as HTMLInputElement;

beforeEach(() => {
  confirmations.length = 0;
  useLLMPoliciesCatalog.mockReturnValue({ data: undefined, isLoading: false, error: null });
  useGuardrailPolicyDefinition.mockReturnValue({ data: undefined, isLoading: false, error: null });
});

afterEach(() => {
  // Release the module-level dialog lock between tests.
  confirmations.forEach((c) => c.onCancel?.());
});

describe("PolicySelectorDrawer", () => {
  it("fetches the org catalog scoped to the provider when no data is supplied", () => {
    useLLMPoliciesCatalog.mockReturnValue({
      data: catalog([inlinePolicy, { ...hubPolicy, name: "excluded" }]),
      isLoading: false,
      error: null,
    });
    renderDrawer({ catalogData: undefined, providerId: "prov-1" });

    expect(useLLMPoliciesCatalog).toHaveBeenCalledWith("acme", true, "prov-1");
    expect(screen.getByText("PII Masking")).toBeInTheDocument();
    // Non-guardrail policies are filtered out of the default catalog.
    expect(screen.queryByText("excluded")).not.toBeInTheDocument();
  });

  it("does not fetch when the caller supplies the catalog", () => {
    renderDrawer();
    expect(useLLMPoliciesCatalog).toHaveBeenCalledWith("acme", false, undefined);
  });

  it("uses a custom filter instead of the guardrail filter", () => {
    renderDrawer({ filterPolicies: (p) => p.filter((x) => x.name === "word-count") });
    expect(screen.getByText("word-count")).toBeInTheDocument();
    expect(screen.queryByText("PII Masking")).not.toBeInTheDocument();
  });

  it("shows the loading and error states", () => {
    const { rerender } = renderDrawer({ isLoadingCatalog: true, loadingLabel: "Loading guardrails..." });
    expect(screen.getByText("Loading guardrails...")).toBeInTheDocument();

    rerender(
      <ThemeProvider theme={createTheme()}>
        <MemoryRouter>
          <PolicySelectorDrawer
            open
            onClose={vi.fn()}
            onSubmit={vi.fn()}
            catalogError={new Error("gateway down")}
            catalogErrorLabel="Could not load."
          />
        </MemoryRouter>
      </ThemeProvider>,
    );
    expect(screen.getByText(/Could not load\. gateway down/)).toBeInTheDocument();
  });

  it("lists policies with versions, truncated descriptions and added ones disabled", () => {
    renderDrawer({ disabledPolicyKeys: ["word-count@2.1.0"] });
    expect(screen.getByText("Choose a policy to configure advanced options.")).toBeInTheDocument();
    expect(screen.getByText("2.1.0")).toBeInTheDocument();
    expect(screen.getByText(`${"x".repeat(200)}...`)).toBeInTheDocument();
    // An already-added policy cannot be picked again.
    fireEvent.click(screen.getByText("word-count"));
    expect(screen.getByPlaceholderText("Search policies")).toBeInTheDocument();
    fireEvent.click(screen.getByText("PII Masking"));
    expect(screen.getByText("* Pattern")).toBeInTheDocument();
  });

  it("disables by policy name when no keys are given", () => {
    renderDrawer({ disabledPolicyNames: ["pii-masking"] });
    fireEvent.click(screen.getByText("PII Masking"));
    expect(screen.queryByText("* Pattern")).not.toBeInTheDocument();
  });

  it("filters by name and description and shows the empty search state", () => {
    renderDrawer({ searchPlaceholder: "Find" });
    const search = screen.getByPlaceholderText("Find");

    fireEvent.change(search, { target: { value: "personal" } });
    expect(screen.getByText("PII Masking")).toBeInTheDocument();
    expect(screen.queryByText("word-count")).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "WORD" } });
    expect(screen.getByText("word-count")).toBeInTheDocument();

    fireEvent.change(search, { target: { value: "nothing-matches" } });
    expect(screen.getByText("No policies match your search")).toBeInTheDocument();
  });

  it("shows the empty catalog state", () => {
    renderDrawer({ catalogData: catalog([]), emptyCatalogTitle: "Nothing here" });
    expect(screen.getByText("Nothing here")).toBeInTheDocument();
  });

  it("configures an inline-schema policy and submits its settings", () => {
    const { onSubmit } = renderDrawer();
    fireEvent.click(screen.getByText("PII Masking"));

    // The inline schema needs no definition fetch.
    expect(useGuardrailPolicyDefinition).toHaveBeenLastCalledWith(undefined, undefined);
    expect(screen.getAllByText("PII Masking").length).toBeGreaterThan(0);
    fireEvent.change(inputByLabel("* Pattern"), { target: { value: "\\d{4}" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(onSubmit).toHaveBeenCalledWith(inlinePolicy, { pattern: "\\d{4}" });
    // Back to the list after submitting.
    expect(screen.getByPlaceholderText("Search policies")).toBeInTheDocument();
  });

  it("fetches a hub policy's YAML definition at the resolved version", () => {
    useGuardrailPolicyDefinition.mockReturnValue({
      data: "name: word-count\nversion: 2.1.0\nparameters:\n  type: object\n  properties:\n    max:\n      type: integer\n",
      isLoading: false,
      error: null,
    });
    renderDrawer({ getPolicyDefinitionVersion: () => "v2" });
    fireEvent.click(screen.getByText("word-count"));

    expect(useGuardrailPolicyDefinition).toHaveBeenLastCalledWith("word-count", "v2");
    expect(screen.getByText("Max")).toBeInTheDocument();
  });

  it.each([
    [
      "loading",
      { data: undefined, isLoading: true, error: null },
      "Loading definition...",
    ],
    [
      "a fetch error",
      { data: undefined, isLoading: false, error: new Error("404 not found") },
      "404 not found",
    ],
    [
      "unparseable YAML",
      { data: "description: no name", isLoading: false, error: null },
      "Failed to parse policy definition.",
    ],
    [
      "no definition",
      { data: undefined, isLoading: false, error: null },
      "No definition available",
    ],
  ])("shows %s for a hub policy", (_label, result, text) => {
    useGuardrailPolicyDefinition.mockReturnValue(result);
    renderDrawer();
    fireEvent.click(screen.getByText("word-count"));
    expect(screen.getByText(text)).toBeInTheDocument();
  });

  it("returns to the list from an error state", () => {
    useGuardrailPolicyDefinition.mockReturnValue({ data: undefined, isLoading: false, error: {} });
    renderDrawer();
    fireEvent.click(screen.getByText("word-count"));
    expect(screen.getByText("Failed to load policy definition.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByPlaceholderText("Search policies")).toBeInTheDocument();
  });

  it("goes back without asking when nothing changed, and confirms when dirty", () => {
    renderDrawer();
    fireEvent.click(screen.getByText("PII Masking"));
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(confirmations).toHaveLength(0);
    expect(screen.getByPlaceholderText("Search policies")).toBeInTheDocument();

    fireEvent.click(screen.getByText("PII Masking"));
    fireEvent.change(inputByLabel("* Pattern"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(confirmations).toHaveLength(1);
    expect(screen.queryByPlaceholderText("Search policies")).not.toBeInTheDocument();
  });

  it("discards without confirmation through Cancel", () => {
    const { onClose } = renderDrawer();
    fireEvent.click(screen.getByText("PII Masking"));
    fireEvent.change(inputByLabel("* Pattern"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(confirmations).toHaveLength(0);
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByPlaceholderText("Search policies")).toBeInTheDocument();
  });

  it("opens straight into the editor for editPolicyKey", () => {
    const { onSubmit } = renderDrawer({
      editPolicyKey: "pii-masking@1.0.0",
      existingSettings: { pattern: "abc" },
    });
    expect(inputByLabel("* Pattern")).toHaveValue("abc");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledWith(inlinePolicy, { pattern: "abc" });
  });

  it("closes from edit mode via Cancel", () => {
    const { onClose } = renderDrawer({ editPolicyKey: "pii-masking@1.0.0" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("closes from the header when clean", () => {
    const { onClose } = renderDrawer();
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(onClose).toHaveBeenCalled();
  });
});
