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

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  templates: { data: undefined as unknown, isLoading: false, error: null as unknown },
  gateways: { error: null as unknown },
  create: { mutate: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListLLMProviderTemplates: () => hoisted.templates,
  useListGateways: () => hoisted.gateways,
  useCreateLLMProvider: () => hoisted.create,
}));

// The real form is covered in shared-component; this stub shows the props the
// page hands it and submits a fixed set of values.
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  AddLLMProviderForm: (props: {
    templates: { id: string; name: string }[];
    isLoadingTemplates: boolean;
    errorMessage: string | null;
    missingParamsMessage: string | null;
    isSubmitting: boolean;
    onSubmit: (values: Record<string, unknown>, guardrails: unknown[]) => void;
    onCancel: () => void;
    onDirtyChange: (dirty: boolean) => void;
  }) => (
    <div>
      <div>{`templates: ${props.templates.map((t) => t.name).join(", ")}`}</div>
      {props.isLoadingTemplates && <div>loading templates</div>}
      {props.errorMessage && <div>{`error: ${props.errorMessage}`}</div>}
      {props.isSubmitting && <div>submitting</div>}
      <button onClick={() => props.onDirtyChange(true)}>edit form</button>
      <button
        onClick={() =>
          props.onSubmit(
            {
              displayName: "My OpenAI",
              version: "v1.0",
              context: "/my-openai",
              templateId: "openai",
              upstreamUrl: "https://api.openai.com/v1",
              apiKey: "sk-123",
              gatewayIds: ["gw-1"],
            },
            [{ name: "pii-masking", version: "v1", settings: { level: "high" } }],
          )
        }
      >
        submit form
      </button>
      <button onClick={props.onCancel}>cancel form</button>
    </div>
  ),
}));

import { renderWithProviders } from "./__test__/renderWithProviders";
import { AddLLMProvidersOrganization } from "./AddLLMProviders.Organization";

const renderPage = (access: Record<string, boolean> = {}) =>
  renderWithProviders(<AddLLMProvidersOrganization />, {
    path: "/org/acme/llm-providers/add",
    route: "/org/:orgId/llm-providers/add",
    access,
  });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.create.mutate.mockReset();
  hoisted.templates = {
    data: {
      templates: [
        {
          id: "openai",
          name: "OpenAI",
          metadata: { auth: { type: "api-key", header: "Authorization", valuePrefix: "Bearer " } },
        },
        { id: "anthropic", name: "Anthropic" },
      ],
    },
    isLoading: false,
    error: null,
  };
  hoisted.gateways = { error: null };
  hoisted.create.isPending = false;
});

describe("AddLLMProvidersOrganization", () => {
  it("offers the organization's provider templates", () => {
    renderPage();
    expect(screen.getByText("Add LLM Service Provider")).toBeInTheDocument();
    expect(screen.getByText("templates: OpenAI, Anthropic")).toBeInTheDocument();
  });

  it("creates the provider from the form values and opens it", () => {
    hoisted.create.mutate.mockImplementation((_args, opts) => opts.onSuccess({ uuid: "new-uuid" }));
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "edit form" }));
    fireEvent.click(screen.getByRole("button", { name: "submit form" }));

    const [args] = hoisted.create.mutate.mock.calls[0];
    expect(args.params).toEqual({ orgName: "acme" });
    expect(args.body).toMatchObject({
      id: "my-openai",
      name: "My OpenAI",
      version: "v1.0",
      context: "/my-openai",
      template: "openai",
      upstream: {
        main: {
          url: "https://api.openai.com/v1",
          auth: { type: "api-key", header: "Authorization", value: "Bearer sk-123" },
        },
      },
      policies: [
        { name: "pii-masking", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { level: "high" } }] },
      ],
      gateways: ["gw-1"],
    });
    expect(screen.getByText("at /org/acme/llm-providers/view/new-uuid")).toBeInTheDocument();
  });

  it("stays on the page when the create fails", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "submit form" }));
    expect(hoisted.create.mutate).toHaveBeenCalled();
    expect(screen.getByText("templates: OpenAI, Anthropic")).toBeInTheDocument();
  });

  it("shows the submitting and loading states", () => {
    hoisted.create.isPending = true;
    hoisted.templates = { data: undefined, isLoading: true, error: null };
    renderPage();
    expect(screen.getByText("submitting")).toBeInTheDocument();
    expect(screen.getByText("loading templates")).toBeInTheDocument();
  });

  it("surfaces template or gateway load errors", () => {
    hoisted.gateways = { error: new Error("gateways unavailable") };
    const { unmount } = renderPage();
    expect(screen.getByText("error: gateways unavailable")).toBeInTheDocument();
    unmount();

    hoisted.templates = { data: undefined, isLoading: false, error: new Error("templates unavailable") };
    renderPage();
    expect(screen.getByText("error: templates unavailable")).toBeInTheDocument();
  });

  it("goes back to the list on cancel", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "cancel form" }));
    expect(screen.getByText("at /org/acme/llm-providers")).toBeInTheDocument();
  });

  it("sends users without write access back to the list", () => {
    renderPage({ llm_provider_write: false });
    expect(screen.getByText("at /org/acme/llm-providers")).toBeInTheDocument();
    expect(screen.queryByText("Add LLM Service Provider")).not.toBeInTheDocument();
  });
});
