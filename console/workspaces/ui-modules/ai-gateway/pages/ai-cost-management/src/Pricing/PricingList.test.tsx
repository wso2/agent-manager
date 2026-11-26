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

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PricingResponse } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  state: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
  },
  usePricing: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  // What the shared list shell reads besides the hook under test.
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  usePricing: (...args: unknown[]) => {
    hoisted.usePricing(...args);
    return hoisted.state;
  },
}));

import { PricingList } from "./PricingList";

const catalog = (over: Partial<PricingResponse> = {}): PricingResponse => ({
  updatedAt: "2026-11-25T08:00:00Z",
  providers: ["azure", "openai"],
  total: 2,
  limit: 25,
  offset: 0,
  models: [
    {
      model: "gpt-4o",
      provider: "openai",
      inputPer1M: "2.50",
      outputPer1M: "10.00",
      cachedInputPer1M: "1.25",
      maxInputTokens: 128000,
      maxOutputTokens: 16384,
    },
    {
      model: "dall-e-2",
      provider: null,
      inputPer1M: null,
      outputPer1M: null,
      cachedInputPer1M: null,
      maxInputTokens: null,
      maxOutputTokens: null,
    },
  ],
  ...over,
});

const renderList = () =>
  render(
    <MemoryRouter>
      <ThemeProvider theme={createTheme()}>
        <PricingList orgName="acme" />
      </ThemeProvider>
    </MemoryRouter>,
  );

const lastCall = () =>
  hoisted.usePricing.mock.calls[hoisted.usePricing.mock.calls.length - 1];

beforeEach(() => {
  hoisted.usePricing.mockClear();
  hoisted.state = { data: catalog(), isLoading: false, error: undefined };
});

describe("PricingList", () => {
  it("shows each model with exact prices, and dashes for missing ones", () => {
    renderList();
    const row = screen.getByText("gpt-4o").closest("tr") as HTMLElement;
    expect(within(row).getByText("openai")).toBeInTheDocument();
    expect(within(row).getByText("$2.50")).toBeInTheDocument();
    expect(within(row).getByText("$10.00")).toBeInTheDocument();
    expect(within(row).getByText("$1.25")).toBeInTheDocument();

    const bare = screen.getByText("dall-e-2").closest("tr") as HTMLElement;
    // Provider, input, output, cached input and context: all missing.
    expect(within(bare).getAllByText("-")).toHaveLength(4);
  });

  it("names its columns and the unit, and says when it was updated", () => {
    renderList();
    for (const name of [
      "Model",
      "Provider",
      "Input",
      "Output",
      "Cached input",
    ]) {
      expect(screen.getByRole("columnheader", { name })).toBeInTheDocument();
    }
    expect(
      screen.getByText(/USD per 1M tokens\. Updated Nov 25, 2026/),
    ).toBeInTheDocument();
  });

  it("asks for the first page of 25 for the organization", () => {
    renderList();
    expect(lastCall()).toEqual([
      { orgName: "acme" },
      {
        search: undefined,
        provider: undefined,
        sort: "provider",
        limit: 25,
        offset: 0,
      },
    ]);
  });

  it("searches by model name once typing pauses", async () => {
    renderList();
    fireEvent.change(screen.getByPlaceholderText("Search models"), {
      target: { value: "  gpt  " },
    });
    await waitFor(() =>
      expect(lastCall()?.[1]).toEqual({
        search: "gpt",
        provider: undefined,
        sort: "provider",
        limit: 25,
        offset: 0,
      }),
    );
  });

  it("filters by provider from the providers the API listed", async () => {
    renderList();
    fireEvent.mouseDown(
      screen.getByRole("combobox", { name: "Filter by provider" }),
    );
    const options = screen.getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["All providers", "azure", "openai"]);
    fireEvent.click(screen.getByRole("option", { name: "azure" }));
    await waitFor(() =>
      expect(lastCall()?.[1]).toMatchObject({ provider: "azure", offset: 0 }),
    );
  });

  it("pages with limit and offset", async () => {
    hoisted.state = {
      data: catalog({ total: 60 }),
      isLoading: false,
      error: undefined,
    };
    renderList();
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    await waitFor(() =>
      expect(lastCall()?.[1]).toMatchObject({ limit: 25, offset: 25 }),
    );
  });

  it("says there is no pricing yet when the catalog is empty", () => {
    hoisted.state = {
      data: catalog({ total: 0, models: [], providers: [], updatedAt: null }),
      isLoading: false,
      error: undefined,
    };
    renderList();
    expect(screen.getByText("No pricing yet")).toBeInTheDocument();
    expect(
      screen.getByText("Gateways send it when they connect."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/USD per 1M tokens/)).not.toBeInTheDocument();
  });

  it("says when a search matches no model", async () => {
    renderList();
    hoisted.state = {
      data: catalog({ total: 0, models: [] }),
      isLoading: false,
      error: undefined,
    };
    fireEvent.change(screen.getByPlaceholderText("Search models"), {
      target: { value: "nothing" },
    });
    expect(await screen.findByText("No models match")).toBeInTheDocument();
  });

  it("shows an error when the catalog cannot be loaded", () => {
    hoisted.state = {
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
    };
    renderList();
    expect(
      screen.getByText("Couldn't load pricing. Reload to try again."),
    ).toBeInTheDocument();
  });
});
