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
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../utils";
import {
  createSelfHostedGateway,
  createSelfHostedGatewayToken,
  deleteSelfHostedGateway,
  listSelfHostedGatewayTokens,
  revokeSelfHostedGatewayToken,
  updateSelfHostedGateway,
} from "./gateways";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const GW = "/api/v1/orgs/acme/self-hosted-gateways";
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
    const body = { displayName: "Edge", environment: "dev", endpoints: ["https://gw.example.com"] };
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
