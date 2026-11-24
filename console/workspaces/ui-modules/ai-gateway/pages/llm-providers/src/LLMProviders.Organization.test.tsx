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
  list: { data: undefined as unknown, isLoading: false, error: null as unknown },
  deleteProvider: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListLLMProviders: () => hoisted.list,
  useDeleteLLMProvider: () => ({ mutate: hoisted.deleteProvider }),
  useListLLMProviderTemplates: () => ({
    data: {
      templates: [
        { id: "openai", name: "OpenAI", metadata: { logoUrl: "https://logos.example.com/openai.svg" } },
        { id: "anthropic", name: "Anthropic" },
      ],
    },
  }),
}));

vi.mock("./subComponents/ViewLLMProvider", () => ({
  ViewLLMProvider: () => <div>provider details page</div>,
}));
vi.mock("./subComponents/EditLLMProvider", () => ({
  EditLLMProvider: () => <div>provider edit page</div>,
}));

import { renderWithProviders } from "./__test__/renderWithProviders";
import { LLMProvidersOrganization } from "./LLMProviders.Organization";

const provider = (n: number, template = "openai") => ({
  uuid: `uuid-${n}`,
  id: `provider-${n}`,
  name: `Provider ${n}`,
  template,
  createdAt: new Date().toISOString(),
});

const renderPage = (path = "/org/acme/llm-providers", access: Record<string, boolean> = {}) =>
  renderWithProviders(<LLMProvidersOrganization />, {
    path,
    route: "/org/:orgId/llm-providers/*",
    access,
  });

const row = (name: string) => screen.getByText(name).closest("tr") as HTMLElement;

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = {
    data: { providers: [provider(1), provider(2, "anthropic")] },
    isLoading: false,
    error: null,
  };
});

describe("LLMProvidersOrganization", () => {
  it("lists providers with their template names and an add action", () => {
    renderPage();
    expect(screen.getByText("LLM Service Providers")).toBeInTheDocument();
    expect(screen.getByText("Provider 1")).toBeInTheDocument();
    expect(within(row("Provider 1")).getByText("OpenAI")).toBeInTheDocument();
    expect(within(row("Provider 1")).getByRole("img", { name: "openai" })).toBeInTheDocument();
    expect(within(row("Provider 2")).getByText("Anthropic")).toBeInTheDocument();

    expect(screen.getByRole("link", { name: "Add service provider" })).toHaveAttribute(
      "href",
      "/org/acme/llm-providers/add",
    );
  });

  it("opens a provider when its row is clicked", () => {
    renderPage();
    fireEvent.click(screen.getByText("Provider 2"));
    expect(screen.getByText("provider details page")).toBeInTheDocument();
  });

  it("filters providers by name or template", () => {
    renderPage();
    const search = screen.getByPlaceholderText("Search providers");
    fireEvent.change(search, { target: { value: "anthropic" } });
    expect(screen.queryByText("Provider 1")).not.toBeInTheDocument();
    expect(screen.getByText("Provider 2")).toBeInTheDocument();

    fireEvent.change(search, { target: { value: "no-such-provider" } });
    expect(screen.getByText("No providers match your search")).toBeInTheDocument();
  });

  it("deletes a provider after confirmation", async () => {
    renderPage();
    fireEvent.mouseEnter(row("Provider 1"));
    fireEvent.click(within(row("Provider 1")).getByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Delete LLM Provider")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() =>
      expect(hoisted.deleteProvider).toHaveBeenCalledWith({ orgName: "acme", providerId: "uuid-1" }),
    );
  });

  it("pages through more than five providers", () => {
    hoisted.list.data = { providers: Array.from({ length: 7 }, (_, i) => provider(i + 1)) };
    renderPage();
    expect(screen.getByText("Provider 5")).toBeInTheDocument();
    expect(screen.queryByText("Provider 6")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(screen.getByText("Provider 6")).toBeInTheDocument();
    expect(screen.queryByText("Provider 1")).not.toBeInTheDocument();
  });

  it("shows the empty state with the add action as its call to action", () => {
    hoisted.list.data = { providers: [] };
    renderPage();
    expect(screen.getByText("No LLM service providers yet")).toBeInTheDocument();
    // Only the empty state carries the CTA; the header action needs items.
    expect(screen.getAllByRole("link", { name: "Add service provider" })).toHaveLength(1);
  });

  it("shows a load error", () => {
    hoisted.list = { data: undefined, isLoading: false, error: new Error("list failed") };
    renderPage();
    expect(screen.getByText("list failed")).toBeInTheDocument();
  });

  it("hides add, delete and edit for read-only users", () => {
    renderPage("/org/acme/llm-providers", { llm_provider_write: false });
    expect(screen.queryByRole("link", { name: "Add service provider" })).not.toBeInTheDocument();
    fireEvent.mouseEnter(row("Provider 1"));
    expect(within(row("Provider 1")).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("routes to the edit page only for writers", () => {
    renderPage("/org/acme/llm-providers/view/uuid-1/edit");
    expect(screen.getByText("provider edit page")).toBeInTheDocument();
  });

  it("sends read-only users and unknown paths back to the list", () => {
    renderPage("/org/acme/llm-providers/view/uuid-1/edit", { llm_provider_write: false });
    expect(screen.queryByText("provider edit page")).not.toBeInTheDocument();
    expect(screen.getByText("Provider 1")).toBeInTheDocument();
  });
});
