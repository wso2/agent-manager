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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";
import type { AgentDocument, CreateAgentRequest } from "@agent-management-platform/types";

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
  agentDocumentFromCreateRequest,
  agentDocumentFromUpdateRequest,
  agentFromDocument,
  agentListFromDocuments,
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
const AGENT = "/api/v1/agents/a%20b";
const IDS = `${AGENT}/identities`;
const P = "p/1";

afterEach(() => vi.resetAllMocks());

type Method = keyof typeof http;

/** Every call: the helper it uses, the path it builds, and the options it passes. */
type Row = [string, Method, () => Promise<unknown>, string, Record<string, unknown>, unknown?];
const doc = { apiVersion: "v", kind: "Agent", metadata: { name: "a", project: P }, spec: {} } as never;
const calls: Row[] = [
  ["getAgent", "httpGET", () => getAgent(params, getToken), AGENT, { searchParams: { project: P } }],
  ["createAgent", "httpPOST", () => createAgent(params, doc, getToken), "/api/v1/agents", {}, doc],
  ["updateAgent", "httpPUT", () => updateAgent(params, doc, getToken),
    AGENT, { searchParams: { project: P } }, doc],
  ["getAgentRoles", "httpGET", () => getAgentRoles(params, { environment: "dev" }, getToken),
    `${AGENT}/roles`, { searchParams: [["project", P], ["environment", "dev"]] }],
  ["getAgentGroups", "httpGET", () => getAgentGroups(params, { environment: "dev" }, getToken),
    `${AGENT}/groups`, { searchParams: [["project", P], ["environment", "dev"]] }],
  ["getAgentIdentity (one env)", "httpGET",
    () => getAgentIdentity(params, { environment: "dev" }, getToken),
    IDS, { searchParams: [["project", P], ["environment", "dev"]] }],
  ["getAgentIdentity (all envs)", "httpGET", () => getAgentIdentity(params, undefined, getToken),
    IDS, { searchParams: [["project", P]] }],
  ["provisionAgentIdentity", "httpPUT",
    () => provisionAgentIdentity(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { project: P, environment: "dev" } }, {}],
  ["regenerateAgentIdentitySecret", "httpPOST",
    () => regenerateAgentIdentitySecret(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { project: P } }, { environment: "dev" }],
  ["retryAgentIdentityProvisioning", "httpPOST",
    () => retryAgentIdentityProvisioning(params, { environment: "dev" }, getToken),
    `${IDS}/retry`, { searchParams: { project: P } }, { environment: "dev" }],
  ["revokeAgentIdentitySecret", "httpDELETE",
    () => revokeAgentIdentitySecret(params, { environment: "dev" }, getToken),
    IDS, { searchParams: { project: P, environment: "dev" } }],
];

describe.each(calls)("%s", (_name, method, call, path, options, body) => {
  it("addresses the agent, names its project and sends the token", async () => {
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
  it("names the project, repeats array filters and drops undefined ones", async () => {
    http.httpGET.mockResolvedValue(json({ list: [] }));

    await listAgents(
      { orgName: "o", projName: "p" },
      { label: ["env:prod", "team:ml"], limit: 10, search: undefined } as never,
      getToken,
    );

    const [path, opts] = http.httpGET.mock.calls[0];
    expect(path).toBe("/api/v1/agents");
    expect(opts).toEqual({
      searchParams: [["label", "env:prod"], ["label", "team:ml"], ["limit", "10"], ["project", "p"]],
      token: "tok",
    });
  });

  it("falls back to the default project, without a token", async () => {
    http.httpGET.mockResolvedValue(json({ list: [] }));
    await listAgents({} as never);
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/agents", {
      searchParams: [["project", "default"]],
      token: undefined,
    });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockResolvedValue(json({ message: "x" }, 500));
    await expect(listAgents({} as never)).rejects.toEqual({ message: "x" });
  });
});

describe("listOrgAgents", () => {
  const agentDoc = (name: string, project: string) =>
    ({ metadata: { name, project }, spec: { displayName: name.toUpperCase() } });

  it("reads every page of the org-wide list and names each agent's project", async () => {
    type Opts = { searchParams: [string, string][] };
    http.httpGET.mockImplementation(async (path: string, opts: Opts) => {
      if (path === "/api/v1/agents") {
        const offset = Number(new Map(opts.searchParams).get("offset"));
        const pages = [[agentDoc("a1", "p1"), agentDoc("a2", "p2")], [agentDoc("a3", "p1")]];
        return json({ list: offset === 0 ? pages[0] : pages[1], pagination: { total: 3 } });
      }
      return json({ list: [{ metadata: { name: "p1" }, spec: { displayName: "Project 1" } }], pagination: { total: 1 } });
    });

    const got = await listOrgAgents({ orgName: "o" }, getToken);

    expect(got.agents).toEqual([
      { name: "a1", displayName: "A1", projectName: "p1", projectDisplayName: "Project 1" },
      { name: "a2", displayName: "A2", projectName: "p2", projectDisplayName: "p2" },
      { name: "a3", displayName: "A3", projectName: "p1", projectDisplayName: "Project 1" },
    ]);
    const agentCalls = http.httpGET.mock.calls.filter(([p]) => p === "/api/v1/agents");
    expect(agentCalls.map(([, o]) => o.searchParams)).toEqual([
      [["limit", "500"], ["offset", "0"]],
      [["limit", "500"], ["offset", "2"]],
    ]);
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockImplementation(async () => json({ code: "FORBIDDEN" }, 403));
    await expect(listOrgAgents({ orgName: "o" }, getToken)).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

describe("createAgent / updateAgent", () => {
  it("sends a copy of the body, never the caller's object", async () => {
    http.httpPOST.mockResolvedValue(json({}));
    const body = { metadata: { name: "a" }, spec: { extensions: { nested: true } } };
    await createAgent({} as never, body as never);
    const sent = http.httpPOST.mock.calls[0][1];
    expect(sent).toEqual(body);
    expect(sent).not.toBe(body);
    expect(sent.spec).not.toBe(body.spec);
  });
});

describe("generateAgentToken", () => {
  it("posts the body with the project and defined query values", async () => {
    http.httpPOST.mockResolvedValue(json({ token: "jwt" }));

    await expect(
      generateAgentToken(params, { expiresIn: "1h" } as never,
        { environment: "dev", other: undefined } as never, getToken),
    ).resolves.toEqual({ token: "jwt" });

    expect(http.httpPOST).toHaveBeenCalledWith(`${AGENT}/token`, { expiresIn: "1h" }, {
      searchParams: { environment: "dev", project: P },
      token: "tok",
    });
  });

  it("posts an empty body when given none", async () => {
    http.httpPOST.mockResolvedValue(json({ token: "jwt" }));
    await generateAgentToken({ projName: "p", agentName: "a" } as never);
    expect(http.httpPOST).toHaveBeenCalledWith(
      "/api/v1/agents/a/token", {}, { searchParams: { project: "p" }, token: undefined },
    );
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpPOST.mockResolvedValue(json({ code: "X" }, 400));
    await expect(generateAgentToken(params)).rejects.toEqual({ code: "X" });
  });
});

describe("deleteAgent", () => {
  it("deletes the encoded agent in its project and resolves to nothing", async () => {
    http.httpDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(deleteAgent(params, getToken)).resolves.toBeUndefined();
    expect(http.httpDELETE).toHaveBeenCalledWith(AGENT, { searchParams: { project: P }, token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpDELETE.mockResolvedValue(json({ code: "CONFLICT" }, 409));
    await expect(deleteAgent(params)).rejects.toEqual({ code: "CONFLICT" });
  });
});

describe("agent-scoped calls without an agent or project", () => {
  it.each([
    ["getAgent", "agentName", () => getAgent({ orgName: "o", projName: "p" } as never)],
    ["deleteAgent", "agentName", () => deleteAgent({ orgName: "o", projName: "p" } as never)],
    ["updateAgent", "agentName", () => updateAgent({ orgName: "o", projName: "p" } as never, {} as never)],
    ["generateAgentToken", "agentName", () => generateAgentToken({ orgName: "o", projName: "p" } as never)],
    ["getAgent", "projName", () => getAgent({ orgName: "o", agentName: "a" } as never)],
    ["getAgentRoles", "projName", () => getAgentRoles({ orgName: "o", agentName: "a" } as never, { environment: "dev" })],
  ])("%s without %s refuses before sending anything", async (_name, missing, call) => {
    await expect(call()).rejects.toThrow(`${missing} is required`);
    Object.values(http).forEach((fn) => expect(fn).not.toHaveBeenCalled());
  });
});

describe("document conversions", () => {
  const full: AgentDocument = {
    apiVersion: "ai-control-plane.choreolabs.com/v1",
    kind: "Agent",
    metadata: {
      name: "support",
      project: "apps",
      uid: "u-1",
      creationTimestamp: "2026-01-02T03:04:05Z",
      createdBy: "user-1",
    },
    spec: {
      displayName: "Support",
      description: "helps",
      provisioning: { type: "external" },
      agentType: { type: "external-agent-api", subType: "a2a-agent" },
      labels: { team: "care" },
      extensions: { "ai-gateway": { modelConfig: [] } },
    },
    status: { state: "Active" },
  };

  it("carries every field the UI reads off the document", () => {
    expect(agentFromDocument(full)).toEqual({
      uuid: "u-1",
      name: "support",
      displayName: "Support",
      description: "helps",
      createdAt: "2026-01-02T03:04:05Z",
      projectName: "apps",
      status: "Active",
      provisioning: { type: "external" },
      agentType: { type: "external-agent-api", subType: "a2a-agent" },
      labels: { team: "care" },
      createdBy: { id: "user-1" },
    });
  });

  it("carries every create field into the document", () => {
    const body: CreateAgentRequest = {
      name: "support",
      displayName: "Support",
      description: "helps",
      provisioning: { type: "external" },
      agentType: { type: "external-agent-api", subType: "a2a-agent" },
      labels: { team: "care" },
      extensions: { "ai-gateway": { modelConfig: [] } },
    };
    const created = agentDocumentFromCreateRequest("apps", body);
    expect(created.metadata).toEqual({ name: "support", project: "apps" });
    for (const key of Object.keys(full.spec) as (keyof typeof full.spec)[]) {
      expect(created.spec[key], key).toEqual(full.spec[key]);
    }
  });

  it("round-trips a created agent back to the flat model", () => {
    const back = agentFromDocument(agentDocumentFromCreateRequest("apps", {
      name: "a", displayName: "A", provisioning: { type: "external" }, agentType: { type: "t" },
    }));
    expect(back).toMatchObject({ name: "a", displayName: "A", projectName: "apps", description: "" });
  });

  it("keeps absent labels apart from cleared ones on update", () => {
    const absent = agentDocumentFromUpdateRequest("apps", "a", { displayName: "A" });
    expect(JSON.parse(JSON.stringify(absent)).spec).toEqual({ displayName: "A" });
    expect(absent.spec.provisioning).toBeUndefined();
    expect(absent.spec.agentType).toBeUndefined();

    const cleared = agentDocumentFromUpdateRequest("apps", "a", { displayName: "A", labels: {} });
    expect(JSON.parse(JSON.stringify(cleared)).spec.labels).toEqual({});
    expect(cleared.metadata).toEqual({ name: "a", project: "apps" });
  });

  it("unwraps a list page into the flat envelope", () => {
    const page = { count: 1, list: [full], pagination: { total: 7, limit: 10, offset: 5 } };
    expect(agentListFromDocuments(page))
      .toMatchObject({ total: 7, limit: 10, offset: 5, agents: [{ name: "support" }] });
    expect(agentListFromDocuments({ list: [] }))
      .toEqual({ agents: [], total: 0, limit: 0, offset: 0 });
  });
});
