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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, type HttpMocks } from "../../test-utils/http-cases";
import {
  assignGatewayToEnvironment,
  createGateway,
  deleteGateway,
  deleteIdentityProvider,
  discoverOidc,
  getGateway,
  listEnvironmentIdentityProviders,
  listGateways,
  listGatewayTokens,
  listIdentityProviders,
  removeGatewayFromEnvironment,
  revokeGatewayToken,
  rotateGatewayToken,
  updateGateway,
  upsertIdentityProvider,
  createSelfHostedGateway,
  createSelfHostedGatewayToken,
  deleteSelfHostedGateway,
  listSelfHostedGatewayTokens,
  revokeSelfHostedGatewayToken,
  updateSelfHostedGateway,
} from "./gateways";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const GW = "/m/ai-gateway/orgs/acme/self-hosted-gateways";
const ID = "3f2c1a9e-0000-4000-8000-000000000001";
const params = { orgName: "acme", gatewayId: ID };
const getToken = async () => "jwt";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const noContent = () => new Response(null, { status: 204 });

beforeEach(() => {
  vi.mocked(httpGET).mockReset().mockImplementation(async () => json({}));
  vi.mocked(httpPOST).mockReset().mockImplementation(async () => json({}, 201));
  vi.mocked(httpPUT).mockReset().mockImplementation(async () => json({}));
  vi.mocked(httpDELETE).mockReset().mockImplementation(async () => noContent());
});

describe("self-hosted gateway API", () => {
  it("registers a gateway with POST", async () => {
    const body = { displayName: "Edge", handle: "edge", environment: "dev", endpoints: ["https://gw.example.com"] };
    await createSelfHostedGateway({ orgName: "acme" }, body, getToken);
    expect(httpPOST).toHaveBeenCalledWith(GW, body, { token: "jwt" });
  });

  it("updates a gateway with PUT", async () => {
    await updateSelfHostedGateway(params, { displayName: "Edge 2" }, getToken);
    expect(httpPUT).toHaveBeenCalledWith(`${GW}/${ID}`, { displayName: "Edge 2" }, { token: "jwt" });
  });

  it("deletes a gateway and accepts 204", async () => {
    await expect(deleteSelfHostedGateway(params, getToken)).resolves.toBeUndefined();
    expect(httpDELETE).toHaveBeenCalledWith(`${GW}/${ID}`, { token: "jwt" });
  });

  it("lists tokens", async () => {
    await listSelfHostedGatewayTokens(params, getToken);
    expect(httpGET).toHaveBeenCalledWith(`${GW}/${ID}/tokens`, { token: "jwt" });
  });

  it("issues a token with POST and returns it", async () => {
    vi.mocked(httpPOST).mockImplementation(async () => json({ token: "secret", tokenId: "t1" }, 201));
    const res = await createSelfHostedGatewayToken(params, getToken);
    expect(httpPOST).toHaveBeenCalledWith(`${GW}/${ID}/tokens`, {}, { token: "jwt" });
    expect(res.token).toBe("secret");
  });

  it("revokes a token and accepts 204", async () => {
    await revokeSelfHostedGatewayToken({ ...params, tokenId: "t1" }, getToken);
    expect(httpDELETE).toHaveBeenCalledWith(`${GW}/${ID}/tokens/t1`, { token: "jwt" });
  });

  it("throws the error body on failure", async () => {
    vi.mocked(httpDELETE).mockImplementation(async () => json({ message: "has deployments" }, 409));
    await expect(deleteSelfHostedGateway(params, getToken)).rejects.toEqual({ message: "has deployments" });
  });
});

const ORG = "/m/ai-gateway/orgs/acme%20corp";
const gw = { orgName: "acme corp", gatewayId: "gw/1" };
const httpMocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

describeHttpCases("managed gateway API", httpMocks, [
  {
    name: "listGateways forwards every filter",
    call: () => listGateways({ orgName: "acme corp" }, {
      limit: 10, offset: 0, type: "managed", status: "ACTIVE", environment: "dev",
    } as never, getToken),
    method: "GET", url: `${ORG}/gateways`,
    searchParams: { limit: "10", offset: "0", type: "managed", status: "ACTIVE", environment: "dev" },
  },
  {
    name: "listGateways without filters sends no search params",
    call: () => listGateways({ orgName: "acme corp" }, {}, getToken),
    method: "GET", url: `${ORG}/gateways`,
  },
  {
    name: "createGateway",
    call: () => createGateway({ orgName: "acme corp" }, { name: "edge" } as never, getToken),
    method: "POST", url: `${ORG}/gateways`, body: { name: "edge" },
  },
  { name: "getGateway", call: () => getGateway(gw, getToken), method: "GET", url: `${ORG}/gateways/gw%2F1` },
  {
    name: "updateGateway",
    call: () => updateGateway(gw, { displayName: "Edge" } as never, getToken),
    method: "PUT", url: `${ORG}/gateways/gw%2F1`, body: { displayName: "Edge" },
  },
  {
    name: "deleteGateway", call: () => deleteGateway(gw, getToken),
    method: "DELETE", url: `${ORG}/gateways/gw%2F1`, returnsBody: false,
  },
  {
    name: "assignGatewayToEnvironment",
    call: () => assignGatewayToEnvironment({ ...gw, envId: "dev 1" }, getToken),
    method: "POST", url: `${ORG}/gateways/gw%2F1/environments/dev%201`, body: {},
  },
  {
    name: "removeGatewayFromEnvironment",
    call: () => removeGatewayFromEnvironment({ ...gw, envId: "dev 1" }, getToken),
    method: "DELETE", url: `${ORG}/gateways/gw%2F1/environments/dev%201`, returnsBody: false,
  },
  {
    name: "listGatewayTokens", call: () => listGatewayTokens(gw, getToken),
    method: "GET", url: `${ORG}/gateways/gw%2F1/tokens`,
  },
  {
    name: "rotateGatewayToken", call: () => rotateGatewayToken(gw, getToken),
    method: "POST", url: `${ORG}/gateways/gw%2F1/tokens`, body: {},
  },
  {
    name: "revokeGatewayToken", call: () => revokeGatewayToken({ ...gw, tokenId: "t 1" }, getToken),
    method: "DELETE", url: `${ORG}/gateways/gw%2F1/tokens/t%201`, returnsBody: false,
  },
  {
    name: "listIdentityProviders", call: () => listIdentityProviders({ orgName: "acme corp" }, getToken),
    method: "GET", url: `${ORG}/identity-providers`,
  },
  {
    name: "discoverOidc passes the discovery URL as a query param",
    call: () => discoverOidc({ orgName: "acme corp" }, { url: "https://idp.example/.well-known" }, getToken),
    method: "GET", url: `${ORG}/identity-providers/discover`,
    searchParams: { url: "https://idp.example/.well-known" },
  },
  {
    name: "listEnvironmentIdentityProviders",
    call: () => listEnvironmentIdentityProviders({ orgName: "acme corp", environmentId: "env/1" }, getToken),
    method: "GET", url: `${ORG}/environments/env%2F1/identity-providers`,
  },
  {
    name: "upsertIdentityProvider",
    call: () => upsertIdentityProvider({ ...gw, name: "my idp" }, { issuer: "https://idp" } as never, getToken),
    method: "PUT", url: `${ORG}/gateways/gw%2F1/identity-providers/my%20idp`, body: { issuer: "https://idp" },
  },
  {
    name: "deleteIdentityProvider",
    call: () => deleteIdentityProvider({ ...gw, name: "my idp" }, getToken),
    method: "DELETE", url: `${ORG}/gateways/gw%2F1/identity-providers/my%20idp`, returnsBody: false,
  },
]);

describe("gateway API required params", () => {
  it("refuses a missing gateway id before any request", async () => {
    await expect(getGateway({ orgName: "acme", gatewayId: "" }, getToken)).rejects.toThrow("gatewayId");
    expect(httpGET).not.toHaveBeenCalled();
  });
});
