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

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  provider: { data: undefined as unknown, isLoading: false, error: null as unknown },
  update: { mutateAsync: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGetLLMProvider: () => hoisted.provider,
  useUpdateLLMProvider: () => hoisted.update,
  useListLLMProviderTemplates: () => ({
    data: {
      templates: [
        {
          id: "openai",
          name: "OpenAI",
          metadata: {
            logoUrl: "https://logos.example.com/openai.svg",
            openapiSpecUrl: "https://specs.example.com/openai.yaml",
            auth: { valuePrefix: "Bearer " },
          },
        },
      ],
    },
  }),
}));

// Each tab has its own tests; these stubs show what the page passes them.
type TabProps = {
  openapiSpecUrl?: string;
  valuePrefix?: string;
  error?: Error | null;
  onUpdate?: (fields: Record<string, unknown>) => Promise<unknown>;
  onGoToDeploymentTab?: () => void;
  onGoToSecurityTab?: () => void;
};
vi.mock("./LLMProviderOverviewTab", () => ({
  LLMProviderOverviewTab: (p: TabProps) => (
    <div>
      <div>{`overview tab spec=${p.openapiSpecUrl} error=${p.error?.message ?? "none"}`}</div>
      <button onClick={() => void p.onUpdate?.({ openapi: "https://new.example.com/spec.json" })}>
        overview update
      </button>
      <button onClick={p.onGoToDeploymentTab}>go to deployment</button>
    </div>
  ),
}));
vi.mock("./LLMProviderConnectionTab", () => ({
  LLMProviderConnectionTab: (p: TabProps) => <div>{`connection tab prefix=[${p.valuePrefix}]`}</div>,
}));
vi.mock("./LLMProviderDeploymentTab", () => ({
  LLMProviderDeploymentTab: () => <div>deployment tab</div>,
}));
vi.mock("./LLMProviderAccessControlTab", () => ({
  LLMProviderAccessControlTab: () => <div>access control tab</div>,
}));
vi.mock("./LLMProviderSecurityTab", () => ({
  LLMProviderSecurityTab: () => <div>security tab</div>,
}));
vi.mock("./LLMProviderAPIKeysTab", () => ({
  LLMProviderAPIKeysTab: (p: TabProps) => (
    <button onClick={p.onGoToSecurityTab}>api keys tab: go to security</button>
  ),
}));
vi.mock("./LLMProviderRateLimitingTab", () => ({
  LLMProviderRateLimitingTab: () => <div>rate limiting tab</div>,
}));
vi.mock("./LLMProviderGuardrailsTab", () => ({
  LLMProviderGuardrailsTab: () => <div>guardrails tab</div>,
}));
vi.mock("./LLMProviderConsumersTab", () => ({
  LLMProviderConsumersTab: () => <div>consumers tab</div>,
}));

import { makeProvider, renderWithProviders } from "../__test__/renderWithProviders";
import { ViewLLMProvider } from "./ViewLLMProvider";

const renderPage = (access: Record<string, boolean> = {}) =>
  renderWithProviders(<ViewLLMProvider />, { access });

const provider = makeProvider({ description: "  Primary OpenAI account  ", createdAt: "2026-01-01T00:00:00Z" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.provider = { data: provider, isLoading: false, error: null };
  hoisted.update.mutateAsync.mockResolvedValue(provider);
});

describe("ViewLLMProvider", () => {
  it("shows the provider's name, description, template and version", () => {
    renderPage();
    expect(screen.getByText("OpenAI Prod")).toBeInTheDocument();
    expect(screen.getByText("Primary OpenAI account")).toBeInTheDocument();
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    expect(screen.getByText("v1.0")).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: "OpenAI" }).length).toBeGreaterThan(0);
    expect(
      screen.getByText("overview tab spec=https://specs.example.com/openai.yaml error=none"),
    ).toBeInTheDocument();
  });

  it("opens the edit page for writers", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Edit LLM provider" }));
    expect(screen.getByText("at /org/acme/llm-providers/view/p-1/edit")).toBeInTheDocument();
  });

  it("hides editing from read-only users", () => {
    renderPage({ llm_provider_write: false });
    expect(screen.queryByRole("button", { name: "Edit LLM provider" })).not.toBeInTheDocument();
  });

  it("switches between tabs", () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Connection" }));
    expect(screen.getByText("connection tab prefix=[Bearer ]")).toBeInTheDocument();
    expect(screen.queryByText(/overview tab/)).not.toBeInTheDocument();

    for (const [tab, content] of [
      ["Access Control", "access control tab"],
      ["Security", "security tab"],
      ["Rate Limiting", "rate limiting tab"],
      ["Guardrails", "guardrails tab"],
      ["Consumers", "consumers tab"],
    ]) {
      fireEvent.click(screen.getByRole("tab", { name: tab }));
      expect(screen.getByText(content)).toBeInTheDocument();
    }
  });

  it("jumps to the tab a panel asks for", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "go to deployment" }));
    expect(screen.getByText("deployment tab")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "API Keys" }));
    fireEvent.click(screen.getByRole("button", { name: "api keys tab: go to security" }));
    expect(screen.getByText("security tab")).toBeInTheDocument();
  });

  it("merges a tab's changes into the full provider when updating", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "overview update" }));
    await waitFor(() =>
      expect(hoisted.update.mutateAsync).toHaveBeenCalledWith({
        params: { orgName: "acme", providerId: "p-1" },
        body: { ...provider, openapi: "https://new.example.com/spec.json" },
      }),
    );
  });

  it("passes a load error to the tabs as an Error", () => {
    hoisted.provider = { data: undefined, isLoading: false, error: { message: "forbidden" } };
    renderPage();
    expect(screen.getByText(/^overview tab .* error=forbidden$/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit LLM provider" })).not.toBeInTheDocument();
  });
});
