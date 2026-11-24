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

import { vi } from "vitest";

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
vi.mock("../apis/gateways", () => ({
  assignGatewayToEnvironment: vi.fn(),
  createSelfHostedGateway: vi.fn(),
  createSelfHostedGatewayToken: vi.fn(),
  deleteSelfHostedGateway: vi.fn(),
  listSelfHostedGatewayTokens: vi.fn(),
  revokeSelfHostedGatewayToken: vi.fn(),
  updateSelfHostedGateway: vi.fn(),
  deleteGateway: vi.fn(),
  deleteIdentityProvider: vi.fn(),
  discoverOidc: vi.fn(),
  getGateway: vi.fn(),
  listEnvironmentIdentityProviders: vi.fn(),
  listGatewayTokens: vi.fn(),
  listGateways: vi.fn(),
  listIdentityProviders: vi.fn(),
  removeGatewayFromEnvironment: vi.fn(),
  revokeGatewayToken: vi.fn(),
  rotateGatewayToken: vi.fn(),
  updateGateway: vi.fn(),
  upsertIdentityProvider: vi.fn(),
}));

import * as api from "../apis/gateways";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  useAssignGatewayToEnvironment,
  useCreateSelfHostedGateway,
  useDeleteGateway,
  useDeleteIdentityProvider,
  useDiscoverOidc,
  useGetGateway,
  useListEnvironmentIdentityProviders,
  useListGateways,
  useListGatewayTokens,
  useListIdentityProviders,
  useRemoveGatewayFromEnvironment,
  useRevokeGatewayToken,
  useRotateGatewayToken,
  useSelfHostedGatewayTokens,
  useUpdateGateway,
  useUpsertIdentityProvider,
} from "./gateways";

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("ai-gateway");
  else modules.installed.clear();
};

const m = vi.mocked(api);
const org = { orgName: "o" };
const gw = { orgName: "o", gatewayId: "g" };
const query = { type: "managed" };

describeQueryHooks("gateway queries", [
  {
    name: "useListGateways",
    use: () => useListGateways(org, query as never),
    api: m.listGateways, args: [org, query], key: ["gateways", org, query],
    idle: [() => useListGateways({ orgName: "" })],
  },
  {
    name: "useGetGateway",
    use: () => useGetGateway(gw),
    api: m.getGateway, args: [gw], key: ["gateway", gw],
    idle: [() => useGetGateway({ ...gw, gatewayId: "" })],
  },
  {
    name: "useListIdentityProviders",
    use: () => useListIdentityProviders(org),
    api: m.listIdentityProviders, args: [org], key: ["identity-providers", org],
    idle: [() => useListIdentityProviders({ orgName: "" })],
  },
  {
    name: "useListEnvironmentIdentityProviders",
    use: () => useListEnvironmentIdentityProviders({ ...org, environmentId: "e" }),
    api: m.listEnvironmentIdentityProviders, args: [{ ...org, environmentId: "e" }],
    key: ["environment-identity-providers", { ...org, environmentId: "e" }],
    idle: [() => useListEnvironmentIdentityProviders({ ...org, environmentId: "" })],
  },
  {
    name: "useListGatewayTokens",
    use: () => useListGatewayTokens(gw),
    api: m.listGatewayTokens, args: [gw], key: ["gateway-tokens", gw],
    idle: [() => useListGatewayTokens({ ...gw, gatewayId: "" })],
  },
  {
    name: "useSelfHostedGatewayTokens",
    use: () => useSelfHostedGatewayTokens(gw),
    api: m.listSelfHostedGatewayTokens, args: [gw], key: ["self-hosted-gateway-tokens", gw],
    idle: [() => useSelfHostedGatewayTokens(gw, { enabled: false }), () => useSelfHostedGatewayTokens({ ...gw, orgName: "" })],
  },
], { setModulePresent });

const SEEDED = {
  gateways: ["gateways", org],
  gateway: ["gateway", gw],
  idps: ["identity-providers", org],
  envIdps: ["environment-identity-providers", { ...org, environmentId: "e" }],
  tokens: ["gateway-tokens", gw],
  selfHostedTokens: ["self-hosted-gateway-tokens", gw],
} as const;

const envParams = { ...gw, envId: "e" };
const idp = { ...gw, name: "idp" };

describeMutationHooks("gateway mutations", SEEDED, [
  {
    name: "useDiscoverOidc", use: () => useDiscoverOidc(org), api: m.discoverOidc,
    variables: { url: "https://idp/.well-known" }, args: [org, { url: "https://idp/.well-known" }], invalidates: [],
  },
  {
    name: "useUpsertIdentityProvider", use: useUpsertIdentityProvider, api: m.upsertIdentityProvider,
    variables: { params: idp, body: { issuer: "i" } }, args: [idp, { issuer: "i" }], invalidates: ["idps", "envIdps"],
  },
  {
    name: "useDeleteIdentityProvider", use: useDeleteIdentityProvider, api: m.deleteIdentityProvider,
    variables: idp, args: [idp], invalidates: ["idps", "envIdps"],
  },
  {
    name: "useUpdateGateway", use: useUpdateGateway, api: m.updateGateway,
    variables: { params: gw, body: { displayName: "G" } }, args: [gw, { displayName: "G" }], invalidates: ["gateways", "gateway"],
  },
  {
    name: "useDeleteGateway", use: useDeleteGateway, api: m.deleteGateway,
    variables: gw, args: [gw], invalidates: ["gateways", "gateway"],
  },
  {
    name: "useAssignGatewayToEnvironment", use: useAssignGatewayToEnvironment, api: m.assignGatewayToEnvironment,
    variables: envParams, args: [envParams], invalidates: ["gateways", "gateway"],
  },
  {
    name: "useRemoveGatewayFromEnvironment", use: useRemoveGatewayFromEnvironment, api: m.removeGatewayFromEnvironment,
    variables: envParams, args: [envParams], invalidates: ["gateways", "gateway"],
  },
  {
    name: "useRotateGatewayToken", use: useRotateGatewayToken, api: m.rotateGatewayToken,
    variables: gw, args: [gw], invalidates: ["tokens"],
  },
  {
    name: "useRevokeGatewayToken", use: useRevokeGatewayToken, api: m.revokeGatewayToken,
    variables: { ...gw, tokenId: "t" }, args: [{ ...gw, tokenId: "t" }], invalidates: ["tokens"],
  },
  {
    name: "useCreateSelfHostedGateway", use: useCreateSelfHostedGateway, api: m.createSelfHostedGateway,
    variables: { params: org, body: { handle: "edge" } }, args: [org, { handle: "edge" }], invalidates: ["gateways", "gateway"],
  },
]);
