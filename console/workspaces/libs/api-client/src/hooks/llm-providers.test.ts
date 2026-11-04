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
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/llm-providers", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  listLLMDeployments: vi.fn(),
}));

import { listLLMDeployments } from "../apis/llm-providers";
import { useListLLMDeployments } from "./llm-providers";
import { POLL_INTERVAL } from "../utils";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", providerId: "p" };

let unmount: (() => void) | undefined;
afterEach(() => unmount?.());

describe("useListLLMDeployments", () => {
  it.each([
    [["DEPLOYED", "DEPLOYING"], POLL_INTERVAL],
    [["UNDEPLOYING"], POLL_INTERVAL],
    [["DEPLOYED", "FAILED", "ARCHIVED"], false],
    [[], false],
  ] as const)("with statuses %j polls at %s", async (statuses, interval) => {
    vi.mocked(listLLMDeployments).mockResolvedValueOnce(
      statuses.map((status) => ({ status })) as never,
    );
    const qc = new QueryClient();
    const Probe = () => {
      useListLLMDeployments(params);
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () =>
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
    );
    unmount = () => act(() => root.unmount());
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    const query = qc.getQueryCache().find({ queryKey: ["llm-deployments", params, undefined] })!;
    const refetchInterval = query.observers[0].options.refetchInterval;
    expect(typeof refetchInterval === "function" ? refetchInterval(query) : refetchInterval).toBe(interval);
  });
});
