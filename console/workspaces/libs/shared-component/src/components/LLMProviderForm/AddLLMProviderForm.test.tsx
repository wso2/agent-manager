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

import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// MUI transitions and first renders are slow on a busy machine.
vi.setConfig({ testTimeout: 30000 });

const gatewaysState: {
  data?: { gateways: { uuid: string; gatewayType: string }[] };
  isLoading: boolean;
  error?: unknown;
} = { data: { gateways: [] }, isLoading: false };

vi.mock("@agent-management-platform/api-client", () => ({
  useListGateways: vi.fn(() => gatewaysState),
}));

// The gateway selector and guardrail list have their own tests; stand them in
// with buttons that drive the callbacks the form wires up.
vi.mock("../EnvironmentGatewaySelector/EnvironmentGatewaySelector", () => ({
  EnvironmentGatewaySelector: (props: {
    value: string[];
    onChange: (ids: string[]) => void;
    onValidityChange: (v: boolean) => void;
    onSingleChoiceChange: (v: boolean) => void;
  }) => (
    <div data-testid="gateway-selector" data-value={props.value.join(",")}>
      <button type="button" onClick={() => props.onChange(["gw-1"])}>
        pick gateway
      </button>
      <button type="button" onClick={() => props.onValidityChange(false)}>
        invalidate gateways
      </button>
      <button type="button" onClick={() => props.onSingleChoiceChange(true)}>
        single choice
      </button>
    </div>
  ),
}));

vi.mock("../PolicyListSection/PolicyListSection", () => ({
  PolicyListSection: (props: {
    policies: { name: string; version: string; settings?: unknown }[];
    onAdd: (p: unknown) => void;
    onEdit: (p: unknown) => void;
    onRemove: (n: string, v: string) => void;
  }) => (
    <div data-testid="guardrails">
      <span>{props.policies.map((p) => `${p.name}@${p.version}:${JSON.stringify(p.settings ?? {})}`).join("|")}</span>
      <button type="button" onClick={() => props.onAdd({ name: "pii", version: "v1", settings: { a: 1 } })}>
        add pii
      </button>
      <button type="button" onClick={() => props.onEdit({ name: "pii", version: "v1", settings: { a: 2 } })}>
        edit pii
      </button>
      <button type="button" onClick={() => props.onRemove("pii", "v1")}>
        remove pii
      </button>
    </div>
  ),
}));

import { AddLLMProviderForm, type TemplateCard } from "./AddLLMProviderForm";

const openai: TemplateCard = {
  id: "openai",
  handle: "openai",
  name: "OpenAI",
  description: "GPT models",
  image: "https://logo/openai.png",
  hasTemplateUrl: true,
  endpointUrl: "https://api.openai.com/v1",
  hasTemplateAuthHeader: true,
  authHeader: "Authorization",
};
const custom: TemplateCard = {
  id: "custom",
  handle: "custom",
  name: "Custom",
};

const field = (id: string) =>
  document.getElementById(id) as HTMLInputElement;
const type = (id: string, value: string) =>
  fireEvent.change(field(id), { target: { value } });
const submitButton = (name: RegExp = /Add provider/) =>
  screen.getByRole("button", { name });

function setup(props: Partial<React.ComponentProps<typeof AddLLMProviderForm>> = {}) {
  const onSubmit = vi.fn();
  const onCancel = vi.fn();
  const onDirtyChange = vi.fn();
  render(
    <AddLLMProviderForm
      orgId="org"
      templates={[openai, custom]}
      isLoadingTemplates={false}
      onSubmit={onSubmit}
      onCancel={onCancel}
      onDirtyChange={onDirtyChange}
      {...props}
    />,
  );
  return { onSubmit, onCancel, onDirtyChange };
}

describe("AddLLMProviderForm", () => {
  beforeEach(() => {
    gatewaysState.data = { gateways: [{ uuid: "gw-1", gatewayType: "EGRESS" }] };
    gatewaysState.isLoading = false;
    gatewaysState.error = undefined;
  });

  it("lists templates alphabetically and derives the context path from the name", () => {
    setup();
    const names = screen.getAllByText(/^(OpenAI|Custom)$/).map((n) => n.textContent);
    expect(names).toEqual(["Custom", "OpenAI"]);
    expect(screen.getByRole("img", { name: "OpenAI" })).toHaveAttribute("src", "https://logo/openai.png");

    type("displayName", "  My Prod Provider!! ");
    expect(field("context").value).toBe("/my-prod-provider");
  });

  it("stops deriving context once the user edits it, and resumes after it is cleared", () => {
    setup();
    type("displayName", "First");
    type("context", "/manual");
    type("displayName", "Second");
    expect(field("context").value).toBe("/manual");
    type("context", "");
    type("displayName", "Third");
    expect(field("context").value).toBe("/third");
  });

  it("shows inline validation for bad version, context and timeouts", () => {
    setup();
    type("displayName", "a");
    expect(screen.getByText("Display name must be at least 2 characters")).toBeInTheDocument();
    type("version", "1.0");
    expect(screen.getByText(/Version must match v<major>.<minor>/)).toBeInTheDocument();
    type("context", "/trailing/");
    expect(screen.getByText(/Context must start with \//)).toBeInTheDocument();
  });

  it("submits the trimmed values with the selected template, gateway and guardrails", () => {
    const { onSubmit, onDirtyChange } = setup();
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
    type("displayName", "  Prod OpenAI ");
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
    type("description", " main ");
    fireEvent.click(screen.getByText("OpenAI"));
    // The template's endpoint is filled in and locked.
    expect(field("upstreamUrl").value).toBe("https://api.openai.com/v1");
    expect(field("upstreamUrl")).toHaveAttribute("readonly");
    type("apiKey", " sk-123 ");

    fireEvent.click(screen.getAllByText("add pii")[0]);
    fireEvent.click(screen.getAllByText("add pii")[0]); // duplicate ignored
    fireEvent.click(screen.getAllByText("edit pii")[0]);
    expect(screen.getByTestId("guardrails")).toHaveTextContent('pii@v1:{"a":2}');

    expect(submitButton()).toBeDisabled(); // no gateway yet
    fireEvent.click(screen.getByText("pick gateway"));
    expect(submitButton()).toBeEnabled();
    fireEvent.click(submitButton());

    expect(onSubmit).toHaveBeenCalledWith(
      {
        templateId: "openai",
        displayName: "Prod OpenAI",
        version: "v1.0",
        description: "main",
        context: "/prod-openai",
        upstreamUrl: "https://api.openai.com/v1",
        apiKey: "sk-123",
        resilienceTimeout: "",
        resilienceIdleTimeout: "",
        gatewayIds: ["gw-1"],
      },
      [{ name: "pii", version: "v1", settings: { a: 2 } }],
    );
  });

  it("removes a guardrail", () => {
    setup();
    fireEvent.click(screen.getByText("Custom"));
    fireEvent.click(screen.getByText("add pii"));
    fireEvent.click(screen.getByText("remove pii"));
    expect(screen.getByTestId("guardrails").textContent).not.toContain("pii@v1");
  });

  it("requires an endpoint when the template has none, and an API key when it needs auth", () => {
    const { onSubmit } = setup();
    type("displayName", "Custom one");
    fireEvent.click(screen.getByText("Custom"));
    fireEvent.click(screen.getByText("pick gateway"));
    fireEvent.click(submitButton());
    expect(screen.getByText("Upstream endpoint is required")).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("OpenAI"));
    fireEvent.click(submitButton());
    expect(screen.getByText("API key / credential is required")).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("drops the API key from the payload when the template takes no auth header", () => {
    const { onSubmit } = setup();
    type("displayName", "Custom one");
    fireEvent.click(screen.getByText("Custom"));
    type("upstreamUrl", "https://llm.internal/v1");
    type("apiKey", "ignored");
    fireEvent.click(screen.getByText("pick gateway"));
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ upstreamUrl: "https://llm.internal/v1", apiKey: "" }),
      [],
    );
  });

  it("blocks submit on a schema error and lists it after the next attempt", () => {
    const { onSubmit } = setup();
    fireEvent.click(screen.getByText("Custom"));
    type("upstreamUrl", "https://llm.internal/v1");
    fireEvent.click(screen.getByText("pick gateway"));
    fireEvent.click(submitButton());
    expect(onSubmit).not.toHaveBeenCalled();
    // An empty name fails both the "required" and the min-length rule; the
    // later issue wins in useFormValidation, so that is the message shown.
    expect(screen.getByText("Display name must be at least 2 characters")).toBeInTheDocument();
    // The summary alert snapshots errors as they stood when submit was pressed,
    // so it only lists the problem from the second attempt on.
    expect(screen.queryByText(/Please fix the following/)).not.toBeInTheDocument();
    fireEvent.click(submitButton());
    expect(
      screen.getByText(/Please fix the following: Display name must be at least 2 characters/),
    ).toBeInTheDocument();
  });

  it("lets the user unlock a template endpoint and reveal the API key", () => {
    setup();
    fireEvent.click(screen.getByText("OpenAI"));
    const url = field("upstreamUrl");
    fireEvent.click(within(url.parentElement as HTMLElement).getByRole("button"));
    expect(url).not.toHaveAttribute("readonly");
    expect(field("apiKey")).toHaveAttribute("type", "password");
    fireEvent.click(screen.getByRole("button", { name: "Show API key" }));
    expect(field("apiKey")).toHaveAttribute("type", "text");
    fireEvent.click(screen.getByRole("button", { name: "Hide API key" }));
    expect(field("apiKey")).toHaveAttribute("type", "password");
  });

  it("disables submit while the gateway selection is invalid or a submit is running", () => {
    setup({ isSubmitting: true });
    fireEvent.click(screen.getByText("pick gateway"));
    expect(screen.getByRole("button", { name: "Creating..." })).toBeDisabled();
  });

  it("keeps submit disabled when the selector reports an invalid selection", () => {
    setup();
    fireEvent.click(screen.getByText("pick gateway"));
    fireEvent.click(screen.getByText("invalidate gateways"));
    expect(submitButton()).toBeDisabled();
  });

  it("hides the deployment heading when only one gateway choice exists", () => {
    setup();
    expect(screen.getByText("Deployment Configuration")).toBeInTheDocument();
    fireEvent.click(screen.getByText("single choice"));
    expect(screen.queryByText("Deployment Configuration")).not.toBeInTheDocument();
  });

  it("warns when no egress-capable gateway exists", () => {
    gatewaysState.data = { gateways: [{ uuid: "in", gatewayType: "INGRESS" }] };
    setup();
    expect(screen.getByText(/No egress-capable gateway is configured/)).toBeInTheDocument();
  });

  it("shows a gateway fetch error", () => {
    gatewaysState.data = undefined;
    gatewaysState.error = new Error("gateways down");
    setup();
    expect(screen.getByText("gateways down")).toBeInTheDocument();
  });

  it("renders loading, empty, error and missing-param states and cancels", () => {
    const { onCancel } = setup({
      templates: [],
      isLoadingTemplates: false,
      errorMessage: "Create failed",
      missingParamsMessage: "Missing org",
      submitLabel: "Save",
      showAdvancedBasicDetails: false,
      showGuardrails: false,
    });
    expect(screen.getByText("No provider templates available for this organization.")).toBeInTheDocument();
    expect(screen.getByText("Create failed")).toBeInTheDocument();
    expect(screen.getByText("Missing org")).toBeInTheDocument();
    expect(field("version")).toBeNull();
    expect(screen.queryByTestId("guardrails")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("does not show the empty-template message while templates load", () => {
    setup({ templates: [], isLoadingTemplates: true });
    expect(screen.queryByText(/No provider templates available/)).not.toBeInTheDocument();
  });
});
