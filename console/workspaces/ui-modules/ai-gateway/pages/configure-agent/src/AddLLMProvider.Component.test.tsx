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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";

type Policy = { name: string; version: string; settings?: Record<string, unknown> };
type Mutation = {
  mutate: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  reset: ReturnType<typeof vi.fn>;
};

const state = vi.hoisted(() => ({
  agent: undefined as unknown,
  envs: { environments: [] as unknown[], isLoading: false },
  existingList: undefined as unknown,
  catalog: undefined as unknown,
  config: { data: undefined as unknown, isLoading: false, isError: false },
  create: undefined as unknown as Mutation,
  update: undefined as unknown as Mutation,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: () => ({ data: state.agent }),
  useListAgentModelConfigs: () => ({ data: state.existingList }),
  useListCatalogLLMProviders: () => ({ data: state.catalog }),
  useListLLMProviderTemplates: () => ({
    data: { templates: [{ id: "openai", name: "OpenAI", metadata: { logoUrl: "/openai.svg" } }] },
  }),
  useGetAgentModelConfig: () => state.config,
  useCreateAgentModelConfig: () => state.create,
  useUpdateAgentModelConfig: () => state.update,
}));

vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useOrgEnvironmentsState: () => state.envs,
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  PolicyListSection: ({
    policies,
    onAdd,
  }: {
    policies: Policy[];
    onAdd: (p: Policy) => void;
  }) => (
    <div>
      {policies.map((p) => <span key={p.name}>{`guardrail ${p.name}`}</span>)}
      <button onClick={() => onAdd({ name: "pii", version: "v1", settings: { mask: true } })}>
        add pii
      </button>
    </div>
  ),
}));

import { AddLLMProviderComponent } from "./AddLLMProvider.Component";

const BASE = "/org/o/project/p/agents/a";

function LocationProbe() {
  const location = useLocation();
  return (
    <div>
      <div data-testid="location">{location.pathname}</div>
      <div data-testid="state">{JSON.stringify(location.state)}</div>
    </div>
  );
}

const renderPage = (path = `${BASE}/llm/add`, canWrite = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ agent_write: canWrite }}>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        initialEntries={[path]}
      >
        <Routes>
          <Route path="/org/:orgId/project/:projectId/agents/:agentId/llm/add" element={<AddLLMProviderComponent />} />
          <Route path="/org/:orgId/project/:projectId/agents/:agentId/llm/:configId/edit" element={<AddLLMProviderComponent />} />
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </FeatureManagementProvider>,
  );

const mutation = (): Mutation => ({
  mutate: vi.fn(),
  isPending: false,
  isError: false,
  error: null,
  reset: vi.fn(),
});

const pickProvider = async (name: string) => {
  fireEvent.click(screen.getByRole("button", { name: "Select a Service Provider" }));
  fireEvent.click(await screen.findByRole("button", { name: `${name}. Click to select` }));
};

beforeEach(() => {
  state.agent = { provisioning: { type: "internal" } };
  state.envs = { environments: [{ name: "dev", displayName: "Development", id: "e1" }], isLoading: false };
  state.existingList = { configs: [{ name: "openai-prov" }] };
  state.catalog = {
    entries: [
      { uuid: "u1", handle: "openai-prov", name: "OpenAI Prov", template: "openai", version: "v1" },
      { uuid: "u2", handle: "other-prov", name: "Other Prov", template: "custom" },
    ],
  };
  state.config = { data: undefined, isLoading: false, isError: false };
  state.create = mutation();
  state.update = mutation();
});

describe("AddLLMProviderComponent", () => {
  it("sends a user without write access back to the configure page", () => {
    renderPage(`${BASE}/llm/add`, false);
    expect(screen.getByTestId("location")).toHaveTextContent(`${BASE}/configure`);
  });

  it("waits for the environments before rendering the form", () => {
    state.envs = { environments: [], isLoading: true };
    renderPage();
    expect(screen.getByText("Add LLM Configuration")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("points to the organization's provider page when the catalog is empty", () => {
    state.catalog = { entries: [] };
    renderPage();
    expect(screen.getByText("No service providers available")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add LLM service provider" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/org/o/llm-providers/add");
  });

  it("keeps Save disabled until a provider is selected", () => {
    renderPage();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.queryByText("Configuration Name")).not.toBeInTheDocument();
  });

  it("names the configuration and its variables after the picked provider", async () => {
    renderPage();
    await pickProvider("OpenAI Prov");
    expect(screen.getByRole("button", { name: "Selected: OpenAI Prov. Click to change." })).toBeInTheDocument();
    // "openai-prov" is taken by an existing configuration.
    expect(screen.getByLabelText("Name")).toHaveValue("openai-prov-2");
    expect(screen.getByDisplayValue("OPENAI_URL")).toBeInTheDocument();
    expect(screen.getByDisplayValue("OPENAI_API_KEY")).toBeInTheDocument();
  });

  it("creates the configuration and opens it with the one-time credentials", async () => {
    state.create.mutate.mockImplementation((_vars, opts: { onSuccess: (d: unknown) => void }) =>
      opts.onSuccess({
        uuid: "cfg-new",
        envMappings: {
          dev: { configuration: { authInfo: { type: "apikey", in: "header", name: "X-Key", value: "sk-1" } } },
        },
      }),
    );
    renderPage();
    await pickProvider("OpenAI Prov");
    fireEvent.click(screen.getByRole("button", { name: "add pii" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "  chat-llm  " } });
    fireEvent.change(screen.getByDisplayValue("OPENAI_API_KEY"), { target: { value: "MY_KEY" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(state.create.mutate).toHaveBeenCalledWith(
      {
        params: { orgName: "o", projName: "p", agentName: "a" },
        body: {
          name: "chat-llm",
          type: "llm",
          envMappings: {
            dev: {
              providerName: "openai-prov",
              configuration: {
                policies: [{ name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] }],
              },
            },
          },
          environmentVariables: [
            { key: "url", name: "OPENAI_URL" },
            { key: "apikey", name: "MY_KEY" },
          ],
        },
      },
      expect.anything(),
    );
    expect(screen.getByTestId("location")).toHaveTextContent("cfg-new");
    expect(screen.getByTestId("state")).toHaveTextContent('"value":"sk-1"');
  });

  it("falls back to the suggested name when the name is cleared", async () => {
    renderPage();
    await pickProvider("OpenAI Prov");
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: " " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.create.mutate.mock.calls[0][0].body.name).toBe("openai-prov-2");
  });

  it("sends no variable names for an external agent", async () => {
    state.agent = { provisioning: { type: "external" } };
    renderPage();
    await pickProvider("Other Prov");
    expect(screen.queryByText("Environment Variable Names")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.create.mutate.mock.calls[0][0].body.environmentVariables).toBeUndefined();
  });

  it("warns that environments without a provider are skipped", async () => {
    state.envs = {
      environments: [
        { name: "dev", displayName: "Development", id: "e1" },
        { name: "prod", displayName: "Production", id: "e2" },
      ],
      isLoading: false,
    };
    renderPage();
    expect(screen.getByText("Select which LLM Service provider to use in each environment.")).toBeInTheDocument();
    await pickProvider("OpenAI Prov");
    expect(screen.getByText(/They will be skipped on save/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /Production/ }));
    expect(screen.getByRole("button", { name: "Select a Service Provider" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(Object.keys(state.create.mutate.mock.calls[0][0].body.envMappings)).toEqual(["dev"]);
  });

  it("shows the server's error when creation fails", () => {
    state.create = { ...mutation(), isError: true, error: new Error("name already exists") };
    renderPage();
    expect(screen.getByText("name already exists")).toBeInTheDocument();
  });

  it("returns to the configure page on cancel", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByTestId("location")).toHaveTextContent(`${BASE}/configure`);
  });

  describe("edit mode", () => {
    const savedConfig = {
      name: "saved-llm",
      environmentVariables: [
        { key: "url", name: "BASE_URL" },
        { key: "apikey", name: "BASE_KEY" },
      ],
      envMappings: {
        dev: {
          configuration: {
            providerUuid: "u1",
            providerName: "openai-prov",
            policies: [{ name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] }],
          },
        },
      },
    };

    it("reports a configuration that failed to load", () => {
      state.config = { data: undefined, isLoading: false, isError: true };
      renderPage(`${BASE}/llm/cfg-9/edit`);
      expect(screen.getByText("Config not found or failed to load.")).toBeInTheDocument();
    });

    it("seeds the form from the saved configuration and updates it", () => {
      state.config = { data: savedConfig, isLoading: false, isError: false };
      state.update.mutate.mockImplementation(
        (_vars, opts: { onSuccess: () => void }) => opts.onSuccess(),
      );
      renderPage(`${BASE}/llm/cfg-9/edit`);
      expect(screen.getByText("Edit LLM Configuration")).toBeInTheDocument();
      expect(screen.getByLabelText("Name")).toHaveValue("saved-llm");
      expect(screen.getByDisplayValue("BASE_URL")).toBeInTheDocument();
      expect(screen.getByText("guardrail pii")).toBeInTheDocument();

      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      expect(state.update.mutate).toHaveBeenCalledWith(
        {
          params: { orgName: "o", projName: "p", agentName: "a", configId: "cfg-9" },
          body: {
            name: "saved-llm",
            envMappings: {
              dev: {
                providerName: "openai-prov",
                configuration: {
                  policies: [{ name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] }],
                },
              },
            },
            environmentVariables: [
              { key: "url", name: "BASE_URL" },
              { key: "apikey", name: "BASE_KEY" },
            ],
          },
        },
        expect.anything(),
      );
      expect(screen.getByTestId("location")).toHaveTextContent(`${BASE}/configure`);
    });

    it("shows the update error with a fallback message", () => {
      state.config = { data: savedConfig, isLoading: false, isError: false };
      state.update = { ...mutation(), isError: true, error: {} };
      renderPage(`${BASE}/llm/cfg-9/edit`);
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });
});
