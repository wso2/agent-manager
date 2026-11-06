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
vi.mock("../apis", () => ({
  updateSelfHostedGateway: vi.fn(async () => ({})),
  deleteSelfHostedGateway: vi.fn(async () => undefined),
  createSelfHostedGatewayToken: vi.fn(async () => ({})),
  revokeSelfHostedGatewayToken: vi.fn(async () => undefined),
}));

import {
  useCreateSelfHostedGatewayToken,
  useDeleteSelfHostedGateway,
  useRevokeSelfHostedGatewayToken,
  useUpdateSelfHostedGateway,
} from "./gateways";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", gatewayId: "g", tokenId: "t" };

const SEEDED = {
  gateways: ["gateways", {}],
  gateway: ["gateway", {}],
  tokens: ["self-hosted-gateway-tokens", { orgName: "o", gatewayId: "g" }],
  otherTokens: ["self-hosted-gateway-tokens", { orgName: "o", gatewayId: "other" }],
} as const;

async function invalidatedAfter(
  useHook: () => { mutateAsync: (v: never) => Promise<unknown> },
  variables: unknown,
): Promise<string[]> {
  const result = {} as { current: ReturnType<typeof useHook> };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const qc = new QueryClient();
  Object.values(SEEDED).forEach((key) => qc.setQueryData(key, {}));
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  await act(async () => { await result.current.mutateAsync(variables as never); });
  return Object.entries(SEEDED)
    .filter(([, key]) => qc.getQueryState(key)?.isInvalidated)
    .map(([name]) => name);
}

describe("self-hosted gateway mutations", () => {
  it("update invalidates the list and detail views, not tokens", async () => {
    expect(await invalidatedAfter(useUpdateSelfHostedGateway, { params, body: {} }))
      .toEqual(["gateways", "gateway"]);
  });

  it("delete invalidates the list and detail views, not tokens", async () => {
    expect(await invalidatedAfter(useDeleteSelfHostedGateway, params))
      .toEqual(["gateways", "gateway"]);
  });

  it("token create invalidates only that gateway's tokens", async () => {
    expect(await invalidatedAfter(useCreateSelfHostedGatewayToken, params)).toEqual(["tokens"]);
  });

  it("token revoke invalidates only that gateway's tokens", async () => {
    expect(await invalidatedAfter(useRevokeSelfHostedGatewayToken, params)).toEqual(["tokens"]);
  });
});
