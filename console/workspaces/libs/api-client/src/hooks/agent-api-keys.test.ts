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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const pushSnackBar = vi.fn();
vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agent-api-keys", () => ({ issueTestAgentAPIKey: vi.fn() }));

import { issueTestAgentAPIKey } from "../apis/agent-api-keys";
import { useIssueTestAgentAPIKey } from "./agent-api-keys";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "acme", projName: "p1", agentName: "orders", envId: "dev" };

let unmount: (() => void) | undefined;
beforeEach(() => {
  vi.mocked(issueTestAgentAPIKey).mockReset();
  pushSnackBar.mockClear();
});
afterEach(() => unmount?.());

async function mount<T>(useHook: () => T) {
  const result = {} as { current: T };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const qc = new QueryClient();
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  unmount = () => act(() => root.unmount());
  return { result, qc };
}

const settle = () => new Promise((r) => setTimeout(r, 20));

describe("useIssueTestAgentAPIKey", () => {
  it("issues on demand through the existing fetch and shows no success snackbar", async () => {
    vi.mocked(issueTestAgentAPIKey).mockResolvedValueOnce(
      { status: "ok", apiKey: "k1", expiresAt: "2026-10-21T10:10:00Z" });
    const { result } = await mount(useIssueTestAgentAPIKey);
    await act(async () => { await result.current.mutateAsync(params); await settle(); });
    expect(issueTestAgentAPIKey).toHaveBeenCalledWith(params, expect.any(Function));
    expect(result.current.data?.apiKey).toBe("k1");
    expect(pushSnackBar).not.toHaveBeenCalled();
  });

  it("shows the standard error snackbar on failure", async () => {
    vi.mocked(issueTestAgentAPIKey).mockRejectedValueOnce(
      Object.assign(new Error("HTTP error! status: 500"), { status: 500 }));
    const { result } = await mount(useIssueTestAgentAPIKey);
    await act(async () => {
      await result.current.mutateAsync(params).catch(() => undefined);
      await settle();
    });
    expect(pushSnackBar).toHaveBeenCalledWith(
      { message: "Failed to submit test API key", type: "error" });
  });
});
