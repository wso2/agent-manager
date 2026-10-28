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
import { describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/deployments", () => ({ updateAgentDeploySettings: vi.fn() }));

import { updateAgentDeploySettings } from "../apis/deployments";
import { a2aProxyQueryKey, agentCardQueryKey } from "./a2a-query-keys";
import { useUpdateAgentDeploySettings } from "./deployments";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", projName: "p", agentName: "a" };
const key = { ...params, envId: "dev" };

describe("useUpdateAgentDeploySettings", () => {
  it("invalidates the gateway routing and card even if the caller unmounts mid-flight", async () => {
    let finish!: () => void;
    vi.mocked(updateAgentDeploySettings).mockImplementationOnce(
      () => new Promise<void>((resolve) => { finish = resolve; }),
    );
    const result = {} as { current: ReturnType<typeof useUpdateAgentDeploySettings> };
    const Probe = () => {
      result.current = useUpdateAgentDeploySettings();
      return null;
    };
    const qc = new QueryClient();
    qc.setQueryData(a2aProxyQueryKey(key), { status: "published" });
    qc.setQueryData(agentCardQueryKey(key), { status: "fetched" });
    const root = createRoot(document.createElement("div"));
    await act(async () =>
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
    );

    let settled = false;
    const save = result.current.mutateAsync({ params, body: { environmentName: "dev" } });
    void save.then(() => { settled = true; });
    await act(async () => root.unmount());

    finish();
    await act(async () => { await save; });

    expect(settled).toBe(true);
    expect(qc.getQueryState(a2aProxyQueryKey(key))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(agentCardQueryKey(key))?.isInvalidated).toBe(true);
  });
});
