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
  listArgs: [] as unknown[],
  keys: { data: undefined as unknown, isLoading: false, isError: false },
  create: { mutateAsync: vi.fn(), isPending: false },
  revoke: { mutate: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListLLMProviderAPIKeys: (args: unknown) => {
    hoisted.listArgs.push(args);
    return hoisted.keys;
  },
  useCreateLLMProviderAPIKey: () => hoisted.create,
  useRevokeLLMProviderAPIKey: () => hoisted.revoke,
}));

// The real manager drives a drawer with date pickers; this stub exposes the
// tab's callbacks and the state it passes down.
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => {
  const { useState } = await import("react");
  const APIKeysManager = (props: {
    keys?: { name: string }[];
    readOnly: boolean;
    emptyDescription: string;
    onCreate: (i: { displayName: string; expiresAt?: string }) => Promise<string | undefined>;
    onRevoke: (name: string) => void;
  }) => {
    const [created, setCreated] = useState<string>();
    return (
    <div>
      <div>{`keys: ${(props.keys ?? []).map((k) => k.name).join(",")}`}</div>
      <div>{props.readOnly ? "read only" : "can manage"}</div>
      <div>{props.emptyDescription}</div>
      <button
        onClick={async () => {
          const key = await props.onCreate({ displayName: "ci key", expiresAt: "2027-01-01" });
          setCreated(key);
        }}
      >
        create
      </button>
      <button onClick={() => props.onRevoke("old-key")}>revoke</button>
      {created && <div>{`created ${created}`}</div>}
    </div>
    );
  };
  return { ...(await importOriginal<object>()), APIKeysManager };
});

import type { LLMProviderResponse } from "@agent-management-platform/types";
import { makeProvider, renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderAPIKeysTab } from "./LLMProviderAPIKeysTab";

const onGoToSecurityTab = vi.fn();

const withKeys = () =>
  makeProvider({ security: { enabled: true, apiKey: { enabled: true, key: "X-API-Key", in: "header" } } });

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { access = {} as Record<string, boolean>, error = null as Error | null, isLoading = false } = {},
) =>
  renderWithProviders(
    <LLMProviderAPIKeysTab
      providerData={providerData}
      orgName="acme"
      providerId="p-1"
      isLoading={isLoading}
      error={error}
      onGoToSecurityTab={onGoToSecurityTab}
    />,
    { access },
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.listArgs = [];
  hoisted.keys = { data: { keys: [{ name: "k1" }, { name: "k2" }] }, isLoading: false, isError: false };
});

describe("LLMProviderAPIKeysTab", () => {
  it("lists the provider's keys when API key auth is enabled", () => {
    renderTab(withKeys());
    expect(screen.getByText("keys: k1,k2")).toBeInTheDocument();
    expect(screen.getByText("can manage")).toBeInTheDocument();
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual({ orgName: "acme", providerId: "p-1" });
  });

  it("creates a key with a generated name and the chosen display name", async () => {
    hoisted.create.mutateAsync.mockResolvedValue({ apiKey: "plain-key" });
    renderTab(withKeys());
    fireEvent.click(screen.getByRole("button", { name: "create" }));

    expect(await screen.findByText("created plain-key")).toBeInTheDocument();
    const call = hoisted.create.mutateAsync.mock.calls[0][0];
    expect(call.params).toEqual({ orgName: "acme", providerId: "p-1" });
    expect(call.body).toMatchObject({ displayName: "ci key", expiresAt: "2027-01-01" });
    expect(call.body.name).toMatch(/^provider-openai-prod-[a-z0-9]+$/);
  });

  it("revokes a key by name", () => {
    renderTab(withKeys());
    fireEvent.click(screen.getByRole("button", { name: "revoke" }));
    expect(hoisted.revoke.mutate).toHaveBeenCalledWith({
      orgName: "acme",
      providerId: "p-1",
      keyName: "old-key",
    });
  });

  it("is read-only without the API key management scope", () => {
    renderTab(withKeys(), { access: { llm_provider_api_key_manage: false } });
    expect(screen.getByText("read only")).toBeInTheDocument();
  });

  it("points to the Security tab when API key auth is disabled, without fetching keys", () => {
    renderTab(makeProvider());
    expect(screen.getByText("API key authentication is disabled")).toBeInTheDocument();
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual({
      orgName: undefined,
      providerId: undefined,
    });
    fireEvent.click(screen.getByRole("button", { name: "Go to Security Tab" }));
    expect(onGoToSecurityTab).toHaveBeenCalled();
  });

  it("shows a load error instead of the disabled message", () => {
    renderTab(undefined, { error: new Error("provider fetch failed") });
    expect(screen.getByText("provider fetch failed")).toBeInTheDocument();
    expect(screen.queryByText("API key authentication is disabled")).not.toBeInTheDocument();
  });

  it("renders nothing without a provider and a placeholder while loading", () => {
    const { container, rerenderWith } = renderTab(undefined);
    expect(container).toBeEmptyDOMElement();
    rerenderWith(
      <LLMProviderAPIKeysTab providerData={withKeys()} orgName="acme" providerId="p-1" isLoading />,
    );
    expect(screen.queryByText("keys: k1,k2")).not.toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });
});
