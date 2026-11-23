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
  provider: {
    data: undefined as unknown,
    isLoading: false,
    error: null as unknown,
    refetch: vi.fn(),
  },
  update: { mutateAsync: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGetLLMProvider: () => hoisted.provider,
  useUpdateLLMProvider: () => hoisted.update,
}));

import { makeProvider, renderWithProviders } from "../__test__/renderWithProviders";
import { EditLLMProvider } from "./EditLLMProvider";

const renderPage = () =>
  renderWithProviders(<EditLLMProvider />, {
    path: "/org/acme/llm-providers/view/p-1/edit",
    route: "/org/:orgId/llm-providers/view/:providerId/edit",
  });

const nameInput = () => screen.getByLabelText("Name");
const descriptionInput = () => screen.getByLabelText("Description (optional)");

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.provider = {
    data: makeProvider({ description: "Main provider" }),
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };
  hoisted.update.isPending = false;
  hoisted.update.mutateAsync.mockResolvedValue(makeProvider());
});

describe("EditLLMProvider", () => {
  it("prefills the provider's name and description", () => {
    renderPage();
    expect(screen.getByText("Edit LLM Provider")).toBeInTheDocument();
    expect(nameInput()).toHaveValue("OpenAI Prod");
    expect(descriptionInput()).toHaveValue("Main provider");
  });

  it("saves trimmed values over the full provider and returns to its page", async () => {
    renderPage();
    fireEvent.change(nameInput(), { target: { value: "  OpenAI Primary " } });
    fireEvent.change(descriptionInput(), { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("at /org/acme/llm-providers/view/p-1")).toBeInTheDocument();
    expect(hoisted.update.mutateAsync).toHaveBeenCalledWith({
      params: { orgName: "acme", providerId: "p-1" },
      body: { ...makeProvider({ description: "Main provider" }), name: "OpenAI Primary", description: undefined },
    });
  });

  it("validates the name as it is typed and on submit", async () => {
    renderPage();
    fireEvent.change(nameInput(), { target: { value: "A" } });
    expect(screen.getByText("Name must be at least 2 characters")).toBeInTheDocument();

    fireEvent.change(nameInput(), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    // The summary above the form repeats the field's message.
    await waitFor(() =>
      expect(screen.getAllByText("Name must be at least 2 characters")).toHaveLength(2),
    );
    expect(hoisted.update.mutateAsync).not.toHaveBeenCalled();
  });

  it("shows the error when the save fails", async () => {
    hoisted.update.mutateAsync.mockRejectedValue(new Error("name already taken"));
    renderPage();
    fireEvent.change(nameInput(), { target: { value: "Duplicate" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("name already taken")).toBeInTheDocument();
  });

  it("returns to the provider on cancel", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/llm-providers/view/p-1")).toBeInTheDocument();
  });

  it("disables the form while saving", () => {
    hoisted.update.isPending = true;
    renderPage();
    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
    expect(nameInput()).toBeDisabled();
  });

  it("offers a retry when the provider fails to load", () => {
    hoisted.provider = { data: undefined, isLoading: false, error: new Error("gone"), refetch: vi.fn() };
    renderPage();
    expect(screen.getByText("Couldn't load this LLM provider")).toBeInTheDocument();
    expect(screen.getByText("gone")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.provider.refetch).toHaveBeenCalled();
  });

  it("shows a placeholder while loading", () => {
    hoisted.provider = { data: undefined, isLoading: true, error: null, refetch: vi.fn() };
    renderPage();
    expect(screen.queryByLabelText("Name")).not.toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this LLM provider")).not.toBeInTheDocument();
  });
});
