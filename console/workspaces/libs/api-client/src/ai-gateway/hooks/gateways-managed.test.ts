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

const modules = vi.hoisted(() => ({ installed: new Set(["ai-gateway"]) }));
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
  useIsUIModuleInstalled: (id: string) => modules.installed.has(id),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/gateways", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../apis/gateways")>()),
  createSelfHostedGateway: vi.fn(),
  deleteGateway: vi.fn(),
  deleteIdentityProvider: vi.fn(),
  discoverOidc: vi.fn(),
  getGateway: vi.fn(),
  listGatewayTokens: vi.fn(),
  listGateways: vi.fn(),
  listIdentityProviders: vi.fn(),
  revokeGatewayToken: vi.fn(),
  rotateGatewayToken: vi.fn(),
  updateSelfHostedGateway: vi.fn(),
  upsertIdentityProvider: vi.fn(),
}));
vi.mock("../../agent-runtime/apis/deployments", () => ({
  listEnvironments: vi.fn(async () => [
    { id: "e1", name: "default", dataplaneRef: "dp", isProduction: false, createdAt: "t" },
  ]),
}));

import type { GatewayDocument } from "@agent-management-platform/types";
import { listEnvironments } from "../../agent-runtime/apis/deployments";
import * as api from "../apis/gateways";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  useCreateSelfHostedGateway,
  useDeleteGateway,
  useDeleteIdentityProvider,
  useDiscoverOidc,
  useGetGateway,
  useListGateways,
  useListIdentityProviders,
  useSelfHostedGatewayTokens,
  useUpsertIdentityProvider,
} from "./gateways";

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("ai-gateway");
  else modules.installed.clear();
};

const m = vi.mocked(api);
const org = { orgName: "o" };
const gw = { orgName: "o", gatewayId: "g" };
const query = { type: "EGRESS" as const };
const env = api.gatewayEnvironment({ id: "e1", name: "default", dataplaneRef: "dp", isProduction: false, createdAt: "t" });
const doc: GatewayDocument = {
  apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "Gateway",
  metadata: { name: "edge", uid: "g" }, spec: { displayName: "Edge", hosting: "SELF_HOSTED" },
};

describeQueryHooks("gateway queries", [
  {
    name: "useListGateways",
    use: () => useListGateways(org, query),
    api: m.listGateways, args: [org, query], key: ["gateways", org, query],
    data: { list: [doc] }, expected: api.gatewayListFromDocuments({ list: [doc] }, [env]),
    idle: [() => useListGateways({ orgName: "" })],
  },
  {
    name: "useGetGateway",
    use: () => useGetGateway(gw),
    api: m.getGateway, args: [gw], key: ["gateway", gw],
    data: doc, expected: api.gatewayFromDocument(doc, [env]),
    idle: [() => useGetGateway({ ...gw, gatewayId: "" })],
  },
  {
    name: "useListIdentityProviders",
    use: () => useListIdentityProviders(org),
    api: m.listIdentityProviders, args: [org], key: ["identity-providers", org],
    idle: [() => useListIdentityProviders({ orgName: "" })],
  },
  {
    name: "useSelfHostedGatewayTokens",
    use: () => useSelfHostedGatewayTokens(gw),
    api: m.listGatewayTokens, args: [gw], key: ["self-hosted-gateway-tokens", gw],
    idle: [() => useSelfHostedGatewayTokens(gw, { enabled: false }), () => useSelfHostedGatewayTokens({ ...gw, orgName: "" })],
  },
], { setModulePresent });

const SEEDED = {
  gateways: ["gateways", org],
  gateway: ["gateway", gw],
  idps: ["identity-providers", org],
  selfHostedTokens: ["self-hosted-gateway-tokens", gw],
} as const;

const idp = { ...gw, name: "idp" };

describeMutationHooks("gateway mutations", SEEDED, [
  {
    name: "useDiscoverOidc", use: () => useDiscoverOidc(org), api: m.discoverOidc,
    variables: { url: "https://idp/.well-known" }, args: [org, { url: "https://idp/.well-known" }], invalidates: [],
  },
  {
    name: "useUpsertIdentityProvider", use: useUpsertIdentityProvider, api: m.upsertIdentityProvider,
    variables: { params: idp, body: { issuer: "i" } }, args: [idp, { issuer: "i" }], invalidates: ["idps"],
  },
  {
    name: "useDeleteIdentityProvider", use: useDeleteIdentityProvider, api: m.deleteIdentityProvider,
    variables: idp, args: [idp], invalidates: ["idps"],
  },
  {
    name: "useDeleteGateway", use: useDeleteGateway, api: m.deleteGateway,
    variables: gw, args: [gw], invalidates: ["gateways", "gateway"],
  },
  {
    name: "useCreateSelfHostedGateway", use: useCreateSelfHostedGateway, api: m.createSelfHostedGateway,
    variables: { params: org, body: { handle: "edge" } }, args: [org, { handle: "edge" }],
    response: doc, expected: api.gatewayFromDocument(doc), invalidates: ["gateways", "gateway"],
  },
]);

describe("gateway environments", () => {
  it("lists gateways without environments when they cannot be read", async () => {
    vi.mocked(listEnvironments)
      .mockRejectedValueOnce({ code: "FORBIDDEN", message: "insufficient permissions" });
    m.listGateways.mockReset().mockResolvedValue({ list: [doc] });
    const qc = new QueryClient();
    let data: unknown;
    const Probe = () => {
      data = useListGateways(org).data;
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () =>
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))));
    await vi.waitFor(() => expect(data).toBeDefined());
    expect(data).toEqual(api.gatewayListFromDocuments({ list: [doc] }));
  });

  it("fails the gateway list when the environments fail for another reason", async () => {
    const outage = { code: "INTERNAL_ERROR", message: "boom" };
    vi.mocked(listEnvironments).mockRejectedValue(outage);
    m.listGateways.mockReset().mockResolvedValue({ list: [doc] });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    let error: unknown;
    const Probe = () => {
      error = useListGateways(org).error;
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () =>
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))));
    await vi.waitFor(() => expect(error).toEqual(outage));
    vi.mocked(listEnvironments).mockReset();
  });
});
