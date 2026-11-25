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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LLMModelProvider, LLMProviderResponse } from "@agent-management-platform/types";
import { LLMProviderModelsTab } from "./LLMProviderModelsTab";

const onUpdate = vi.fn();

const provider = (modelProviders?: LLMModelProvider[]): LLMProviderResponse => ({
  uuid: "p-1",
  id: "openai-prod",
  name: "OpenAI Prod",
  version: "v1",
  context: "/openai",
  template: "openai",
  upstream: {},
  modelProviders,
});

const openai: LLMModelProvider = {
  id: "openai",
  name: "OpenAI",
  models: [{ id: "gpt-4o", name: "gpt-4o" }],
};

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { error = null as Error | null, isLoading = false } = {},
) =>
  render(
    <LLMProviderModelsTab
      providerData={providerData}
      error={error}
      isLoading={isLoading}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
  );

const modelInput = () => screen.getByPlaceholderText("e.g., gpt-4.1-mini");

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(provider());
});

describe("LLMProviderModelsTab", () => {
  it("adds a model provider from the drawer", async () => {
    renderTab(provider([]));
    expect(screen.getByText("No providers added yet")).toBeInTheDocument();
    expect(screen.getByText("Select a provider to see models")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add model provider" }));
    expect(await screen.findByText("Add Model Provider")).toBeInTheDocument();
    const add = screen.getByRole("button", { name: "Add" });
    expect(add).toBeDisabled();
    fireEvent.click(screen.getByText("Anthropic"));
    fireEvent.click(add);

    await screen.findByText("Model provider added successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      modelProviders: [{ id: "anthropic", name: "Anthropic", models: [] }],
    });
  });

  it("lists the selected provider's models and adds one on Enter", async () => {
    renderTab(provider([openai]));
    expect(screen.getByText("Models Available — OpenAI")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    // Only one model provider is supported.
    expect(screen.getByRole("button", { name: "Add model provider" })).toBeDisabled();

    fireEvent.change(modelInput(), { target: { value: " gpt-4.1-mini " } });
    fireEvent.keyDown(modelInput(), { key: "Enter" });

    await screen.findByText("Model added successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      modelProviders: [
        { ...openai, models: [...(openai.models ?? []), { id: "gpt-4.1-mini", name: "gpt-4.1-mini" }] },
      ],
    });
    expect(modelInput()).toHaveValue("");
  });

  it("rejects an empty or duplicate model id", async () => {
    renderTab(provider([openai]));
    fireEvent.keyDown(modelInput(), { key: "Enter" });
    expect(await screen.findByText("Model id is required")).toBeInTheDocument();

    fireEvent.change(modelInput(), { target: { value: "GPT-4O" } });
    fireEvent.keyDown(modelInput(), { key: "Enter" });
    expect(
      await screen.findByText("This model already exists for the selected provider."),
    ).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("removes a model", async () => {
    renderTab(provider([openai]));
    const chip = screen.getByText("gpt-4o").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(chip.querySelector(".MuiChip-deleteIcon") as Element);

    await screen.findByText("Model removed successfully.");
    expect(onUpdate).toHaveBeenCalledWith({ modelProviders: [{ ...openai, models: [] }] });
  });

  it("removes a model provider", async () => {
    renderTab(provider([openai]));
    fireEvent.click(screen.getByRole("button", { name: "Remove provider" }));
    await screen.findByText("Model provider removed successfully.");
    expect(onUpdate).toHaveBeenCalledWith({ modelProviders: [] });
  });

  it("reports failed updates", async () => {
    onUpdate.mockRejectedValue(new Error("x"));
    renderTab(provider([openai]));
    fireEvent.click(screen.getByRole("button", { name: "Remove provider" }));
    expect(await screen.findByText("Failed to remove model provider.")).toBeInTheDocument();

    fireEvent.change(modelInput(), { target: { value: "o3" } });
    fireEvent.keyDown(modelInput(), { key: "Enter" });
    expect(await screen.findByText("Failed to add model.")).toBeInTheDocument();

    const chip = screen.getByText("gpt-4o").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(chip.querySelector(".MuiChip-deleteIcon") as Element);
    expect(await screen.findByText("Failed to remove model.")).toBeInTheDocument();
  });

  it("reports a failed provider add and lets the drawer be cancelled", async () => {
    onUpdate.mockRejectedValue(new Error("x"));
    renderTab(provider([]));
    fireEvent.click(screen.getByRole("button", { name: "Add model provider" }));
    fireEvent.click(await screen.findByText("Groq"));
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(await screen.findByText("Failed to add model provider.")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Add Model Provider")).not.toBeInTheDocument());
  });

  it("shows the load error, nothing without a provider, and placeholders while loading", () => {
    const { container, rerender } = renderTab(undefined, { error: new Error("load failed") });
    expect(screen.getByText("load failed")).toBeInTheDocument();
    rerender(
      <LLMProviderModelsTab providerData={undefined} onUpdate={onUpdate} isUpdating={false} />,
    );
    expect(container).toBeEmptyDOMElement();
    rerender(
      <LLMProviderModelsTab
        providerData={provider()}
        isLoading
        onUpdate={onUpdate}
        isUpdating={false}
      />,
    );
    expect(screen.queryByText("Model Provider")).not.toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });
});
