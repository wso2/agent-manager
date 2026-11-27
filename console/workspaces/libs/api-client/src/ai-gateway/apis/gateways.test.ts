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

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GatewayDocument } from "@agent-management-platform/types";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, type HttpMocks } from "../../test-utils/http-cases";
import {
  createSelfHostedGateway,
  deleteGateway,
  deleteIdentityProvider,
  discoverOidc,
  gatewayEnvironment,
  gatewayFromDocument,
  gatewayListFromDocuments,
  getGateway,
  listGateways,
  listGatewayTokens,
  listIdentityProviders,
  revokeGatewayToken,
  rotateGatewayToken,
  selfHostedGatewayDocument,
  updateSelfHostedGateway,
  upsertIdentityProvider,
} from "./gateways";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const GW = "/m/ai-gateway/gateways";
const ID = "3f2c1a9e-0000-4000-8000-000000000001";
const params = { orgName: "acme", gatewayId: ID };
const getToken = async () => "jwt";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const noContent = () => new Response(null, { status: 204 });

const stored: GatewayDocument = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "Gateway",
  metadata: { name: "edge", uid: ID, creationTimestamp: "2026-01-01T00:00:00Z", updateTimestamp: "2026-01-02T00:00:00Z" },
  spec: {
    displayName: "Edge", description: "rack 4", hosting: "SELF_HOSTED", type: "EGRESS",
    endpoint: "https://gw.example.com", isCritical: false, properties: { site: "berlin" },
  },
  status: { state: "ACTIVE" },
};

beforeEach(() => {
  vi.mocked(httpGET).mockReset().mockImplementation(async () => json({}));
  vi.mocked(httpPOST).mockReset().mockImplementation(async () => json({}, 201));
  vi.mocked(httpPUT).mockReset().mockImplementation(async () => json({}));
  vi.mocked(httpDELETE).mockReset().mockImplementation(async () => noContent());
});

describe("gateway documents", () => {
  it("flattens a document for the pages, with the organization's environments", () => {
    const env = gatewayEnvironment({ id: "e1", name: "default", dataplaneRef: "dp", isProduction: false, createdAt: "t" });
    expect(gatewayFromDocument(stored, [env])).toEqual({
      uuid: ID, organizationName: "", name: "edge", displayName: "Edge", description: "rack 4",
      gatewayType: "EGRESS", hosting: "SELF_HOSTED", vhost: "https://gw.example.com", runtimeUrl: undefined,
      isCritical: false, properties: { site: "berlin" }, status: "ACTIVE",
      createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-02T00:00:00Z", environments: [env],
    });
    expect(env).toMatchObject({ id: "e1", name: "default", displayName: "default", dnsPrefix: "", updatedAt: "t" });
  });

  it("defaults the fields a bare document leaves out", () => {
    const bare = gatewayFromDocument({ ...stored, metadata: { name: "g" }, spec: { displayName: "G" }, status: undefined });
    expect(bare).toMatchObject({
      uuid: "", gatewayType: "EGRESS", hosting: "MANAGED", vhost: "", isCritical: false, status: "INACTIVE",
      createdAt: "", updatedAt: "", environments: [],
    });
  });

  it("reads the list envelope", () => {
    const list = gatewayListFromDocuments({ count: 1, list: [stored], pagination: { limit: 20, offset: 0, total: 7 } });
    expect(list.gateways).toHaveLength(1);
    expect(list).toMatchObject({ total: 7, limit: 20, offset: 0 });
    expect(gatewayListFromDocuments({ list: [stored] })).toMatchObject({ total: 1, limit: 1, offset: 0 });
  });

  it("builds a self-hosted registration from the add-gateway form", () => {
    expect(selfHostedGatewayDocument({
      displayName: "Edge", handle: "edge", endpoints: ["https://gw.example.com"], isCritical: true,
    })).toEqual({
      apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "Gateway", metadata: { name: "edge" },
      spec: {
        displayName: "Edge", description: undefined, hosting: "SELF_HOSTED", endpoint: "https://gw.example.com",
        isCritical: true, properties: undefined,
      },
    });
  });
});

describe("self-hosted gateway writes", () => {
  it("registers a gateway with a POSTed document", async () => {
    const body = { displayName: "Edge", handle: "edge", endpoints: ["https://gw.example.com"] };
    await createSelfHostedGateway({ orgName: "acme" }, body, getToken);
    expect(httpPOST).toHaveBeenCalledWith(GW, selfHostedGatewayDocument(body), { token: "jwt" });
  });

  it("updates by laying the changes over the stored document", async () => {
    vi.mocked(httpGET).mockImplementation(async () => json(stored));
    await updateSelfHostedGateway(params, { displayName: "Edge 2", isCritical: true }, getToken);
    expect(httpGET).toHaveBeenCalledWith(`${GW}/${ID}`, { token: "jwt" });
    expect(httpPUT).toHaveBeenCalledWith(`${GW}/${ID}`, {
      apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "Gateway", metadata: { name: "edge" },
      spec: { ...stored.spec, displayName: "Edge 2", isCritical: true },
    }, { token: "jwt" });
  });

  it("keeps the stored label when the update leaves it out", async () => {
    vi.mocked(httpGET).mockImplementation(async () => json(stored));
    await updateSelfHostedGateway(params, { description: "rack 5" }, getToken);
    const sent = vi.mocked(httpPUT).mock.calls[0][1] as GatewayDocument;
    expect(sent.spec).toMatchObject({ displayName: "Edge", description: "rack 5" });
  });

  it("throws the error body on failure", async () => {
    vi.mocked(httpDELETE).mockImplementation(async () => json({ message: "has deployments" }, 409));
    await expect(deleteGateway(params, getToken)).rejects.toEqual({ message: "has deployments" });
  });
});

const gw = { orgName: "acme corp", gatewayId: "gw/1" };
const httpMocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

describeHttpCases("gateway API", httpMocks, [
  {
    name: "listGateways forwards every filter",
    call: () => listGateways({ orgName: "acme corp" }, {
      limit: 10, offset: 0, type: "EGRESS", status: "ACTIVE", hosting: "SELF_HOSTED",
    }, getToken),
    method: "GET", url: GW,
    searchParams: { limit: "10", offset: "0", type: "EGRESS", status: "ACTIVE", hosting: "SELF_HOSTED" },
  },
  {
    name: "listGateways without filters sends no search params",
    call: () => listGateways({ orgName: "acme corp" }, {}, getToken),
    method: "GET", url: GW,
  },
  { name: "getGateway", call: () => getGateway(gw, getToken), method: "GET", url: `${GW}/gw%2F1` },
  {
    name: "deleteGateway", call: () => deleteGateway(gw, getToken),
    method: "DELETE", url: `${GW}/gw%2F1`, returnsBody: false,
  },
  {
    name: "listGatewayTokens", call: () => listGatewayTokens(gw, getToken),
    method: "GET", url: `${GW}/gw%2F1/tokens`,
  },
  {
    name: "rotateGatewayToken", call: () => rotateGatewayToken(gw, getToken),
    method: "POST", url: `${GW}/gw%2F1/tokens`, body: {},
  },
  {
    name: "revokeGatewayToken", call: () => revokeGatewayToken({ ...gw, tokenId: "t 1" }, getToken),
    method: "DELETE", url: `${GW}/gw%2F1/tokens/t%201`, returnsBody: false,
  },
  {
    name: "listIdentityProviders", call: () => listIdentityProviders({ orgName: "acme corp" }, getToken),
    method: "GET", url: "/m/ai-gateway/identity-providers",
  },
  {
    name: "discoverOidc passes the discovery URL as a query param",
    call: () => discoverOidc({ orgName: "acme corp" }, { url: "https://idp.example/.well-known" }, getToken),
    method: "GET", url: "/m/ai-gateway/identity-providers/discover",
    searchParams: { url: "https://idp.example/.well-known" },
  },
  {
    name: "upsertIdentityProvider",
    call: () => upsertIdentityProvider({ ...gw, name: "my idp" }, { issuer: "https://idp" } as never, getToken),
    method: "PUT", url: `${GW}/gw%2F1/identity-providers/my%20idp`, body: { issuer: "https://idp" },
  },
  {
    name: "deleteIdentityProvider",
    call: () => deleteIdentityProvider({ ...gw, name: "my idp" }, getToken),
    method: "DELETE", url: `${GW}/gw%2F1/identity-providers/my%20idp`, returnsBody: false,
  },
]);

describe("gateway API required params", () => {
  it("refuses a missing gateway id before any request", async () => {
    await expect(getGateway({ orgName: "acme", gatewayId: "" }, getToken)).rejects.toThrow("gatewayId");
    expect(httpGET).not.toHaveBeenCalled();
  });
});
