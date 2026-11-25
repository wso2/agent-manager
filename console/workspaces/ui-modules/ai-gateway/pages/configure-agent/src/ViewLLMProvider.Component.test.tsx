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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";

type Policy = {
  name: string;
  version: string;
  displayName?: string;
  settings?: Record<string, unknown>;
};

const state = vi.hoisted(() => ({
  agent: undefined as unknown,
  config: { data: undefined as unknown, isLoading: false, isError: false },
  envs: { environments: [] as unknown[], isLoading: false, isError: false },
  catalog: undefined as unknown,
  templates: undefined as unknown,
  guardrailCatalog: undefined as unknown,
  update: {
    mutate: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
    isPending: false,
    isError: false,
    error: null as unknown,
    reset: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
  },
  keys: { data: undefined as unknown, isLoading: false, isError: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: () => ({ data: state.agent }),
  useGetAgentModelConfig: () => state.config,
  useLLMPoliciesCatalog: () => ({ data: state.guardrailCatalog }),
  useListCatalogLLMProviders: () => ({ data: state.catalog }),
  useListLLMProviderTemplates: () => ({ data: state.templates }),
  useUpdateAgentModelConfig: () => state.update,
  useListLLMConfigAPIKeys: () => state.keys,
  useCreateLLMConfigAPIKey: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRotateLLMConfigAPIKey: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRevokeLLMConfigAPIKey: () => ({ mutate: vi.fn(), isPending: false }),
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useOrgEnvironmentsState: () => state.envs,
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  CodeBlock: ({ code }: { code: string }) => <pre data-testid="code">{code}</pre>,
  // The guardrail picker has its own catalog fetching; a stub exposes the
  // callbacks the page wires to it.
  PolicyListSection: ({
    policies,
    onAdd,
    onRemove,
    readOnly,
  }: {
    policies: Policy[];
    onAdd: (p: Policy) => void;
    onRemove: (name: string, version: string) => void;
    readOnly?: boolean;
  }) => (
    <div data-testid="guardrails">
      {policies.map((p) => (
        <div key={p.name}>
          <span>{`guardrail ${p.displayName ?? p.name}`}</span>
          {!readOnly && (
            <button onClick={() => onRemove(p.name, p.version)}>{`remove ${p.name}`}</button>
          )}
        </div>
      ))}
      {!readOnly && (
        <button
          onClick={() => onAdd({ name: "toxicity", version: "v2", settings: { level: 3 } })}
        >
          add toxicity
        </button>
      )}
    </div>
  ),
}));

import { ViewLLMProviderComponent } from "./ViewLLMProvider.Component";

const ROUTE = "/org/:orgId/project/:projectId/agents/:agentId/llm/:configId";

const baseConfig = () => ({
  name: "my-llm",
  environmentVariables: [
    { key: "url", name: "OPENAI_URL" },
    { key: "apikey", name: "OPENAI_API_KEY" },
  ],
  envMappings: {
    dev: {
      configuration: {
        providerName: "openai-prov",
        providerUuid: "u1",
        url: "https://gw.example/openai",
        authInfo: { type: "apikey", in: "header", name: "X-API-Key" },
        policies: [
          { name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] },
        ],
        providerPolicies: [{ name: "rate", version: "v1" }],
      },
    },
  },
});

function LocationProbe() {
  return <div data-testid="location">{useLocation().pathname}</div>;
}

const renderPage = (opts: { state?: unknown; canWrite?: boolean } = {}) =>
  render(
    <FeatureManagementProvider featureAccess={{ agent_write: opts.canWrite ?? true }}>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        initialEntries={[{ pathname: "/org/o/project/p/agents/a/llm/cfg-1", state: opts.state }]}
      >
        <Routes>
          <Route path={ROUTE} element={<ViewLLMProviderComponent />} />
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </FeatureManagementProvider>,
  );

beforeEach(() => {
  state.agent = { provisioning: { type: "internal" } };
  state.config = { data: baseConfig(), isLoading: false, isError: false };
  state.envs = {
    environments: [{ name: "dev", displayName: "Development", id: "e1" }],
    isLoading: false,
    isError: false,
  };
  state.catalog = {
    entries: [
      { uuid: "u1", handle: "openai-prov", name: "OpenAI Prov", template: "openai", version: "v1", policies: ["rate"] },
      { uuid: "u2", handle: "anthropic-prov", name: "Anthropic Prov", template: "anthropic", version: "v1", policies: ["jailbreak"] },
    ],
  };
  state.templates = { templates: [{ id: "openai", name: "OpenAI", metadata: {} }] };
  state.guardrailCatalog = { data: [{ name: "rate", displayName: "Rate Guard" }, { name: "pii", displayName: "PII Masking" }] };
  state.update = { mutate: vi.fn(), isPending: false, isError: false, error: null, reset: vi.fn() };
  state.keys = { data: { keys: [] }, isLoading: false, isError: false };
});

describe("ViewLLMProviderComponent", () => {
  it("shows a skeleton while the configuration loads", () => {
    state.config = { data: undefined, isLoading: true, isError: false };
    renderPage();
    expect(screen.getByText("LLM Configuration")).toBeInTheDocument();
    expect(screen.queryByTestId("guardrails")).not.toBeInTheDocument();
  });

  it("reports a failure to load the environments", () => {
    state.envs = { environments: [], isLoading: false, isError: true };
    renderPage();
    expect(screen.getByText(/Failed to load the organization's environments/)).toBeInTheDocument();
  });

  it("reports a missing configuration", () => {
    state.config = { data: undefined, isLoading: false, isError: true };
    renderPage();
    expect(screen.getByText("Configuration not found or failed to load.")).toBeInTheDocument();
  });

  it("shows the provider, its saved guardrails and the provider's own guardrails", () => {
    renderPage();
    expect(screen.getByRole("heading", { name: "my-llm" })).toBeInTheDocument();
    expect(screen.getByText("OpenAI Prov")).toBeInTheDocument();
    expect(screen.getByText("guardrail PII Masking")).toBeInTheDocument();
    // Provider-level guardrails are labelled from the catalog.
    expect(screen.getAllByText("Rate Guard").length).toBeGreaterThan(0);
    // Nothing edited yet, so no save bar.
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("saves a removed guardrail with the provider and variable names unchanged", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "remove pii" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.update.mutate).toHaveBeenCalledWith(
      {
        params: { orgName: "o", projName: "p", agentName: "a", configId: "cfg-1" },
        body: {
          name: "my-llm",
          envMappings: {
            dev: {
              providerName: "openai-prov",
              configuration: { policies: undefined, resilience: undefined },
            },
          },
          environmentVariables: [
            { key: "url", name: "OPENAI_URL" },
            { key: "apikey", name: "OPENAI_API_KEY" },
          ],
        },
      },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("saves an added guardrail as a policy covering every path", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "add toxicity" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(body.envMappings.dev.configuration.policies).toEqual([
      { name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] },
      { name: "toxicity", version: "v2", paths: [{ path: "/*", methods: ["*"], params: { level: 3 } }] },
    ]);
  });

  it("rejects a malformed timeout and saves a valid one", () => {
    renderPage();
    const timeout = screen.getByLabelText("Request Timeout");
    fireEvent.change(timeout, { target: { value: "soon" } });
    fireEvent.blur(timeout);
    expect(screen.getByText("Enter a duration like 5s, 500ms, or 1m")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.update.mutate).not.toHaveBeenCalled();

    fireEvent.change(timeout, { target: { value: "30s" } });
    fireEvent.blur(timeout);
    expect(screen.queryByText("Enter a duration like 5s, 500ms, or 1m")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(body.envMappings.dev.configuration.resilience).toEqual({ timeout: "30s", idleTimeout: undefined });
  });

  it("switches provider and drops the guardrails picked for the old one", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Change provider" }));
    fireEvent.click(await screen.findByRole("button", { name: "Anthropic Prov. Click to select" }));
    expect(screen.queryByText("guardrail PII Masking")).not.toBeInTheDocument();
    // The new provider's own guardrails replace the old provider's.
    expect(screen.getAllByText("jailbreak").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(body.envMappings.dev).toEqual({
      providerName: "anthropic-prov",
      configuration: { policies: undefined, resilience: undefined },
    });
  });

  it("cancels edits and returns to the configure page's LLM tab", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "remove pii" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/org/o/project/p/agents/a/configure");
  });

  it("is read-only without agent write access", () => {
    renderPage({ canWrite: false });
    expect(screen.getByText("OpenAI Prov")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Change provider" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Request Timeout")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove pii/ })).not.toBeInTheDocument();
  });

  it("offers to pick a provider for an environment without one", () => {
    state.envs = {
      environments: [
        { name: "dev", displayName: "Development", id: "e1" },
        { name: "prod", displayName: "Production", id: "e2" },
      ],
      isLoading: false,
      isError: false,
    };
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Production" }));
    expect(screen.getByText("No provider is configured for this environment yet.")).toBeInTheDocument();
    expect(screen.getByText("No org-level guardrails configured.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Select provider" }));
    expect(screen.getByText("Select LLM Service")).toBeInTheDocument();
  });

  it("saves a provider picked for a previously empty environment", async () => {
    state.envs = {
      environments: [
        { name: "dev", displayName: "Development", id: "e1" },
        { name: "prod", displayName: "Production", id: "e2" },
      ],
      isLoading: false,
      isError: false,
    };
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Production" }));
    fireEvent.click(screen.getByRole("button", { name: "Select provider" }));
    fireEvent.click(await screen.findByRole("button", { name: "Anthropic Prov. Click to select" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(Object.keys(body.envMappings).sort()).toEqual(["dev", "prod"]);
    expect(body.envMappings.prod.providerName).toBe("anthropic-prov");
    // The untouched environment keeps its stored policies.
    expect(body.envMappings.dev.configuration.policies).toHaveLength(1);
  });

  it("shows the save error returned by the server", () => {
    state.update = { ...state.update, isError: true, error: new Error("quota exceeded") };
    renderPage();
    expect(screen.getByText("quota exceeded")).toBeInTheDocument();
  });

  it("guides an internal agent with a Python snippet that sends the gateway header", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
    const code = screen.getByTestId("code").textContent ?? "";
    expect(code).toContain("from openai import OpenAI");
    expect(code).toContain("url = os.environ.get('OPENAI_URL')");
    expect(code).toContain('default_headers={"X-API-Key": apikey}');
    fireEvent.click(screen.getByRole("button", { name: "Copilot prompt" }));
    const prompt = screen.getByTestId("code").textContent ?? "";
    expect(prompt).toContain("Update my code to use OpenAI.");
    expect(prompt).toContain("Send the API key in the `X-API-Key` request header");
  });

  it("renames an injected variable from the guide and saves it", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
    fireEvent.change(screen.getByDisplayValue("OPENAI_URL"), { target: { value: "LLM_BASE" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(body.environmentVariables).toEqual([
      { key: "url", name: "LLM_BASE" },
      { key: "apikey", name: "OPENAI_API_KEY" },
    ]);
  });

  it("does not fabricate header code for a proxy that takes its key in the query", () => {
    const cfg = baseConfig();
    cfg.envMappings.dev.configuration.authInfo = { type: "apikey", in: "query", name: "key" };
    state.config = { data: cfg, isLoading: false, isError: false };
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
    expect(screen.getByText(/query parameter \(e\.g\./)).toBeInTheDocument();
    const code = screen.getByTestId("code").textContent ?? "";
    expect(code).not.toContain("OpenAI(");
    fireEvent.click(screen.getByRole("button", { name: "Copilot prompt" }));
    expect(screen.getByTestId("code").textContent).toContain("query parameter");
  });

  it.each([
    ["anthropic", "from anthropic import Anthropic"],
    ["azure-openai", "azure_endpoint=url"],
    ["mistralai", "async_client=httpx.AsyncClient(headers=_headers)"],
    ["gemini", "_http_options = types.HttpOptions(base_url=url"],
    ["awsbedrock", 'request.headers["X-API-Key"] = apikey'],
  ])("builds a %s client snippet", (template, expected) => {
    state.catalog = {
      entries: [{ uuid: "u1", handle: "openai-prov", name: "Prov", template }],
    };
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
    expect(screen.getByTestId("code").textContent).toContain(expected);
  });

  describe("external agent", () => {
    beforeEach(() => {
      state.agent = { provisioning: { type: "external" } };
    });

    it("opens the connect panel with the one-time key handed over after creation", () => {
      renderPage({
        state: { authInfoByEnv: { dev: { type: "apikey", in: "header", name: "X-API-Key", value: "sk-once" } } },
      });
      expect(screen.getByText("Connect to LLM Provider", { selector: "h1,h2,h3,h4,h5,h6,span,p,div" })).toBeInTheDocument();
      expect(screen.getByText(/copy your API key now/)).toBeInTheDocument();
      expect(screen.getByDisplayValue("https://gw.example/openai")).toBeInTheDocument();
      expect(screen.getByDisplayValue("X-API-Key")).toBeInTheDocument();
      const curl = screen.getByTestId("code").textContent ?? "";
      expect(curl).toContain("curl -X POST 'https://gw.example/openai/chat/completions'");
      expect(curl).toContain('--header "X-API-Key: sk-once"');
      expect(curl).toContain('"model": "gpt-4o-mini"');
    });

    it("sends a query-parameter key with --url-query", () => {
      const cfg = baseConfig();
      cfg.envMappings.dev.configuration.authInfo = { type: "apikey", in: "query", name: "api key" };
      state.config = { data: cfg, isLoading: false, isError: false };
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Connect to LLM Provider" }));
      expect(screen.getByText(/Credentials are only displayed once/)).toBeInTheDocument();
      const curl = screen.getByTestId("code").textContent ?? "";
      expect(curl).toContain('--url-query "api%20key=$OPENAI_API_KEY"');
      expect(curl).not.toContain("--header \"api key");
    });

    it("explains a proxy that needs no credential", () => {
      const cfg = baseConfig();
      delete (cfg.envMappings.dev.configuration as { authInfo?: unknown }).authInfo;
      state.config = { data: cfg, isLoading: false, isError: false };
      state.catalog = { entries: [{ uuid: "u1", handle: "openai-prov", name: "Prov", template: "anthropic" }] };
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Connect to LLM Provider" }));
      expect(screen.getByText(/This proxy requires no credential/)).toBeInTheDocument();
      const curl = screen.getByTestId("code").textContent ?? "";
      expect(curl).toContain("/v1/messages");
      expect(curl).toContain("anthropic-version: 2023-06-01");
      expect(curl).not.toContain("<header-name>");
    });

    it("manages the configuration's gateway API key", () => {
      renderPage();
      expect(screen.getByText(/Only one key can exist per configuration/)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Generate" })).toBeInTheDocument();
    });

    it("does not offer the API key section to an internal agent", () => {
      state.agent = { provisioning: { type: "internal" } };
      renderPage();
      expect(screen.queryByText(/Only one key can exist/)).not.toBeInTheDocument();
    });
  });

  it("keeps the stored mapping for a deleted environment out of the update", async () => {
    type Config = ReturnType<typeof baseConfig> & { envMappings: Record<string, unknown> };
    const cfg = baseConfig() as Config;
    cfg.envMappings["0f3e-uuid"] = { configuration: { providerName: "old-prov" } };
    state.config = { data: cfg, isLoading: false, isError: false };
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "remove pii" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.update.mutate).toHaveBeenCalled());
    const [{ body }] = vi.mocked(state.update.mutate).mock.calls[0];
    expect(Object.keys(body.envMappings)).toEqual(["dev"]);
  });
});
