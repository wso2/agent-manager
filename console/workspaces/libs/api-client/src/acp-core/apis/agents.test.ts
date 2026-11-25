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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));

import {
  createAgent,
  deleteAgent,
  generateAgentToken,
  getAgent,
  getAgentGroups,
  getAgentIdentity,
  getAgentRoles,
  listAgents,
  listOrgAgents,
  provisionAgentIdentity,
  regenerateAgentIdentitySecret,
  retryAgentIdentityProvisioning,
  revokeAgentIdentitySecret,
  updateAgent,
} from "./agents";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const params = { orgName: "my org", projName: "p/1", agentName: "a b" };
const AGENT = "/api/v1/orgs/my%20org/projects/p%2F1/agents/a%20b";
const IDS = `${AGENT}/identities`;

afterEach(() => vi.resetAllMocks());

type Method = keyof typeof http;

/** Every call: the helper it uses, the path it builds, and the options it passes. */
type Row = [string, Method, () => Promise<unknown>, string, Record<string, unknown>, unknown?];
const calls: Row[] = [
  ["listOrgAgents", "httpGET", () => listOrgAgents({ orgName: "my org" }, getToken),
    "/api/v1/orgs/my%20org/agents", {}],
  ["getAgent", "httpGET", () => getAgent(params, getToken), AGENT, {}],
  ["createAgent", "httpPOST",
    () => createAgent(params, { name: "a", displayName: "A" } as never, getToken),
    "/api/v1/orgs/my%20org/projects/p%2F1/agents", {}, { name: "a", displayName: "A" }],
  ["updateAgent", "httpPUT", () => updateAgent(params, { description: "d" } as never, getToken),
    AGENT, {}, { description: "d" }],
  ["getAgentRoles", "httpGET", () => getAgentRoles(params, { environment: "dev" }, getToken),
    `${AGENT}/roles`, { searchParams: { environment: "dev" } }],
  ["getAgentGroups", "httpGET", () => getAgentGroups(params, { environment: "dev" }, getToken),
    `${AGENT}/groups`, { searchParams: { environment: "dev" } }],
  ["getAgentIdentity (one env)", "httpGET",
    () => getAgentIdentity(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { environment: "dev" } }],
  ["getAgentIdentity (all envs)", "httpGET", () => getAgentIdentity(params, undefined, getToken),
    IDS, { searchParams: undefined }],
  ["provisionAgentIdentity", "httpPUT",
    () => provisionAgentIdentity(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { environment: "dev" } }, {}],
  ["regenerateAgentIdentitySecret", "httpPOST",
    () => regenerateAgentIdentitySecret(params, { environment: "dev" } as never, getToken),
    IDS, {}, { environment: "dev" }],
  ["retryAgentIdentityProvisioning", "httpPOST",
    () => retryAgentIdentityProvisioning(params, { environment: "dev" } as never, getToken),
    `${IDS}/retry`, {}, { environment: "dev" }],
  ["revokeAgentIdentitySecret", "httpDELETE",
    () => revokeAgentIdentitySecret(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { environment: "dev" } }],
];

describe.each(calls)("%s", (_name, method, call, path, options, body) => {
  it("sends the request with encoded path params and the token, and returns the body", async () => {
    http[method].mockResolvedValue(json({ result: "ok" }));

    await expect(call()).resolves.toEqual({ result: "ok" });

    const args = http[method].mock.calls[0];
    expect(args[0]).toBe(path);
    if (body !== undefined) expect(args[1]).toEqual(body);
    expect(args[args.length - 1]).toEqual({ token: "tok", ...options });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(call()).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

describe("listAgents", () => {
  it("repeats array filters and drops undefined ones", async () => {
    http.httpGET.mockResolvedValue(json({ agents: [] }));

    await listAgents(
      { orgName: "o", projName: "p" },
      { label: ["env:prod", "team:ml"], limit: 10, search: undefined } as never,
      getToken,
    );

    const [path, opts] = http.httpGET.mock.calls[0];
    expect(path).toBe("/api/v1/orgs/o/projects/p/agents");
    expect(opts).toEqual({
      searchParams: [["label", "env:prod"], ["label", "team:ml"], ["limit", "10"]],
      token: "tok",
    });
  });

  it("falls back to the default org and project, without a token", async () => {
    http.httpGET.mockResolvedValue(json({ agents: [] }));
    await listAgents({} as never);
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/orgs/default/projects/default/agents", {
      searchParams: [],
      token: undefined,
    });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockResolvedValue(json({ message: "x" }, 500));
    await expect(listAgents({} as never)).rejects.toEqual({ message: "x" });
  });
});

describe("createAgent / updateAgent", () => {
  it("sends a copy of the body, never the caller's object", async () => {
    http.httpPOST.mockResolvedValue(json({}));
    const body = { name: "a", config: { nested: true } };
    await createAgent({} as never, body as never);
    const sent = http.httpPOST.mock.calls[0][1];
    expect(sent).toEqual(body);
    expect(sent).not.toBe(body);
    expect(sent.config).not.toBe(body.config);
  });
});

describe("generateAgentToken", () => {
  it("posts the body with defined query values as strings", async () => {
    http.httpPOST.mockResolvedValue(json({ token: "jwt" }));

    await expect(
      generateAgentToken(params, { expiresIn: "1h" } as never,
        { environment: "dev", other: undefined } as never, getToken),
    ).resolves.toEqual({ token: "jwt" });

    expect(http.httpPOST).toHaveBeenCalledWith(`${AGENT}/token`, { expiresIn: "1h" }, {
      searchParams: { environment: "dev" },
      token: "tok",
    });
  });

  it("posts an empty body and no query when given neither", async () => {
    http.httpPOST.mockResolvedValue(json({ token: "jwt" }));
    await generateAgentToken({ agentName: "a" } as never);
    expect(http.httpPOST).toHaveBeenCalledWith(
      "/api/v1/orgs/default/projects/default/agents/a/token", {}, { searchParams: undefined, token: undefined },
    );
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpPOST.mockResolvedValue(json({ code: "X" }, 400));
    await expect(generateAgentToken({ agentName: "a" } as never)).rejects.toEqual({ code: "X" });
  });
});

describe("deleteAgent", () => {
  it("deletes the encoded agent and resolves to nothing", async () => {
    http.httpDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(deleteAgent(params, getToken)).resolves.toBeUndefined();
    expect(http.httpDELETE).toHaveBeenCalledWith(AGENT, { token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpDELETE.mockResolvedValue(json({ code: "CONFLICT" }, 409));
    await expect(deleteAgent(params)).rejects.toEqual({ code: "CONFLICT" });
  });
});

describe("agent-scoped calls without an agent name", () => {
  it.each([
    ["getAgent", () => getAgent({ orgName: "o", projName: "p" } as never)],
    ["deleteAgent", () => deleteAgent({ orgName: "o", projName: "p" } as never)],
    ["updateAgent", () => updateAgent({ orgName: "o", projName: "p" } as never, {} as never)],
    ["generateAgentToken", () => generateAgentToken({ orgName: "o", projName: "p" } as never)],
  ])("%s refuses before sending anything", async (_name, call) => {
    await expect(call()).rejects.toThrow("agentName is required");
    Object.values(http).forEach((fn) => expect(fn).not.toHaveBeenCalled());
  });
});
