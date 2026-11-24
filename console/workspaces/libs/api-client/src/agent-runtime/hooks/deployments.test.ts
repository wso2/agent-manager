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
import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/deployments", () => ({
  updateAgentDeploySettings: vi.fn(),
  listAgentDeployments: vi.fn(),
  getAgentConfigurations: vi.fn(),
  listEnvironments: vi.fn(),
  getEnvironment: vi.fn(),
}));

import * as api from "../apis/deployments";
import { updateAgentDeploySettings } from "../apis/deployments";
import { a2aProxyQueryKey, agentCardQueryKey } from "./a2a-query-keys";
import {
  useGetAgentConfigurations,
  useGetEnvironment,
  useListAgentDeployments,
  useListEnvironments,
  useUpdateAgentDeploySettings,
} from "./deployments";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import { cleanupHooks, flush, renderHook } from "../../test-utils/query-hooks";
import { POLL_INTERVAL } from "../../shared/utils";

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
    qc.setQueryData(a2aProxyQueryKey(key), { status: "DEPLOYED" });
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

const m = vi.mocked(api);

describeQueryHooks("deployment queries", [
  {
    name: "useListAgentDeployments",
    use: () => useListAgentDeployments(params),
    api: m.listAgentDeployments, args: [params], key: ["agent-deployments", "o", "p", "a"],
    idle: [
      () => useListAgentDeployments({ ...params, agentName: "" }),
      // The caller's flag wins over complete params.
      () => useListAgentDeployments(params, { enabled: false }),
      // ...but cannot send a request that is missing the agent's path.
      () => useListAgentDeployments({ ...params, agentName: "" }, { enabled: true }),
    ],
  },
  {
    name: "useGetAgentConfigurations",
    use: () => useGetAgentConfigurations(params, { environment: "dev" }),
    api: m.getAgentConfigurations, args: [params, { environment: "dev" }],
    key: ["agent-configurations", params, { environment: "dev" }],
    idle: [() => useGetAgentConfigurations(params, { environment: "" })],
  },
  {
    name: "useListEnvironments",
    use: () => useListEnvironments({ orgName: "o" }),
    api: m.listEnvironments, args: [{ orgName: "o" }], key: ["environments", { orgName: "o" }],
    idle: [() => useListEnvironments({ orgName: "" })],
  },
  {
    name: "useGetEnvironment",
    use: () => useGetEnvironment({ orgName: "o", envName: "dev" }),
    api: m.getEnvironment, args: [{ orgName: "o", envName: "dev" }],
    key: ["environment", { orgName: "o", envName: "dev" }],
    idle: [() => useGetEnvironment({ orgName: "o", envName: "" })],
  },
]);

describe("useListAgentDeployments polling", () => {
  afterEach(() => cleanupHooks());

  it.each([
    [{ dev: { status: "in-progress" }, prod: { status: "success" } }, POLL_INTERVAL],
    [{ dev: { status: "success" } }, false],
  ])("with %j polls at %s", async (deployments, interval) => {
    m.listAgentDeployments.mockReset().mockResolvedValue(deployments as never);
    const { qc } = await renderHook(() => useListAgentDeployments(params));
    await flush();
    const query = qc.getQueryCache().find({ queryKey: ["agent-deployments", "o", "p", "a"] })!;
    const refetchInterval = query.observers[0].options.refetchInterval;
    expect(typeof refetchInterval === "function" ? refetchInterval(query) : refetchInterval).toBe(interval);
  });
});

describeMutationHooks("deploy settings mutation", {
  agent: ["agent", params],
  configurations: ["agent-configurations", params],
  deployments: ["agent-deployments", "o", "p", "a"],
  devProxy: a2aProxyQueryKey(key),
  prodProxy: a2aProxyQueryKey({ ...key, envId: "prod" }),
  environments: ["environments", { orgName: "o" }],
}, [
  {
    name: "useUpdateAgentDeploySettings", use: useUpdateAgentDeploySettings, api: m.updateAgentDeploySettings,
    variables: { params, body: { environmentName: "dev" } }, args: [params, { environmentName: "dev" }],
    invalidates: ["agent", "configurations", "deployments", "devProxy"], voidResult: true,
  },
]);
