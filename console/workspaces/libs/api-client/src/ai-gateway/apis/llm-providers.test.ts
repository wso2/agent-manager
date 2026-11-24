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
import type { LLMProviderDocument, UpdateLLMProviderRequest } from "@agent-management-platform/types";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, type HttpMocks } from "../../test-utils/http-cases";
import {
  createLLMDeployment,
  createLLMProviderAPIKey,
  createLLMProviderTemplate,
  createLLMProxy,
  createLLMProxyAPIKey,
  deleteLLMDeployment,
  deleteLLMProviderTemplate,
  deleteLLMProxy,
  getLLMDeployment,
  getLLMProviderTemplate,
  getLLMProxy,
  listAvailableLLMPolicies,
  listLLMDeployments,
  listLLMProviderAPIKeys,
  listLLMProviderConsumers,
  listLLMProviderProxies,
  listLLMProviderTemplates,
  listLLMProxies,
  listLLMProxyAPIKeys,
  restoreLLMDeployment,
  revokeLLMProviderAPIKey,
  revokeLLMProxyAPIKey,
  rotateLLMProviderAPIKey,
  rotateLLMProxyAPIKey,
  undeployLLMProvider,
  updateLLMProviderTemplate,
  updateLLMProxy,
  createLLMProvider,
  deleteLLMProvider,
  documentFromCreateRequest,
  documentFromUpdateRequest,
  getLLMProvider,
  listLLMProviders,
  providerFromDocument,
  providerListFromDocuments,
  updateLLMProvider,
} from "./llm-providers";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const BASE = "/m/ai-gateway/llm-providers";
const getToken = async () => "jwt";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

const upstream = {
  main: {
    url: "https://api.openai.com/v1",
    auth: { type: "api-key", header: "Authorization", value: "***REDACTED***" },
  },
};

const doc: LLMProviderDocument = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "LlmProvider",
  metadata: {
    name: "openai-prod",
    uid: "3f2c1a9e-0000-4000-8000-000000000001",
    creationTimestamp: "2026-11-01T10:00:00Z",
    updateTimestamp: "2026-11-02T10:00:00Z",
    createdBy: "admin",
  },
  spec: {
    displayName: "OpenAI Prod",
    version: "v1.0",
    context: "/openai",
    template: "openai",
    upstream,
    description: "primary",
    security: { enabled: true },
    policies: [],
    modelProviders: [],
    gateways: ["gw-1"],
    inCatalog: true,
  },
  status: { gateways: [{ gatewayId: "gw-1", gatewayName: "Edge", status: "DEPLOYED" }] },
} as unknown as LLMProviderDocument;

beforeEach(() => {
  vi.mocked(httpGET).mockReset().mockImplementation(async () => json(doc));
  vi.mocked(httpPOST).mockReset().mockImplementation(async () => json(doc, 201));
  vi.mocked(httpPUT).mockReset().mockImplementation(async () => json(doc));
  const noContent = () => new Response(null, { status: 204 });
  vi.mocked(httpDELETE).mockReset().mockImplementation(async () => noContent());
});

describe("LLM provider API", () => {
  it("lists from the collection, with no org in the path", async () => {
    const page = { list: [doc], pagination: { total: 1, limit: 20, offset: 0 } };
    vi.mocked(httpGET).mockImplementation(async () => json(page));
    await listLLMProviders({ orgName: "acme" }, { limit: 20, offset: 0 }, getToken);
    expect(httpGET).toHaveBeenCalledWith(BASE, {
      token: "jwt",
      searchParams: { limit: "20", offset: "0" },
    });
  });

  it("gets, updates and deletes by provider id", async () => {
    const params = { orgName: "acme", providerId: "openai-prod" };
    await getLLMProvider(params, getToken);
    expect(httpGET).toHaveBeenCalledWith(`${BASE}/openai-prod`, { token: "jwt" });

    const body = documentFromUpdateRequest("openai-prod", { name: "OpenAI" });
    await updateLLMProvider(params, body, getToken);
    expect(httpPUT).toHaveBeenCalledWith(`${BASE}/openai-prod`, body, { token: "jwt" });

    await expect(deleteLLMProvider(params, getToken)).resolves.toBeUndefined();
    expect(httpDELETE).toHaveBeenCalledWith(`${BASE}/openai-prod`, { token: "jwt" });
  });

  it("creates by posting a document", async () => {
    const body = documentFromCreateRequest({
      id: "openai-prod", name: "OpenAI Prod", version: "v1.0", context: "/openai", template: "openai", upstream,
    } as never);
    await createLLMProvider({ orgName: "acme" }, body, getToken);
    expect(httpPOST).toHaveBeenCalledWith(BASE, body, { token: "jwt" });
  });

  it("throws the server's error body on a failure", async () => {
    vi.mocked(httpGET).mockImplementation(async () => json({ message: "LLM provider not found" }, 404));
    await expect(getLLMProvider({ orgName: "acme", providerId: "x" }, getToken)).rejects.toEqual({
      message: "LLM provider not found",
    });
  });
});

describe("provider document conversion", () => {
  it("maps the §5.2 rename and the metadata onto the flat provider", () => {
    const p = providerFromDocument(doc);
    expect(p).toMatchObject({
      uuid: "3f2c1a9e-0000-4000-8000-000000000001",
      id: "openai-prod",
      name: "OpenAI Prod",
      createdBy: "admin",
      createdAt: "2026-11-01T10:00:00Z",
      updatedAt: "2026-11-02T10:00:00Z",
      inCatalog: true,
      gateways: ["gw-1"],
      upstream,
      security: { enabled: true },
    });
  });

  it("carries every spec field across, so none is dropped on the way to the UI", () => {
    const p = providerFromDocument(doc) as unknown as Record<string, unknown>;
    const renamed: Record<string, string> = { displayName: "name" };
    for (const [key, value] of Object.entries(doc.spec)) {
      expect(p[renamed[key] ?? key], key).toEqual(value);
    }
  });

  it("reads list pagination, falling back to the page when the server omits it", () => {
    const paged = { list: [doc], pagination: { total: 7, limit: 1, offset: 3 } };
    expect(providerListFromDocuments(paged)).toMatchObject({ total: 7, limit: 1, offset: 3 });
    const unpaged = providerListFromDocuments({ list: [doc] });
    expect(unpaged).toMatchObject({ total: 1, limit: 1, offset: 0 });
  });

  it("builds a create document with the handle as metadata.name", () => {
    const d = documentFromCreateRequest({
      id: "openai-prod",
      name: "OpenAI Prod",
      version: "v1.0",
      context: "/openai",
      template: "openai",
      upstream,
      gateways: ["gw-1"],
    } as never);
    expect(d.apiVersion).toBe("ai-control-plane.choreolabs.com/v1");
    expect(d.kind).toBe("LlmProvider");
    expect(d.metadata).toEqual({ name: "openai-prod" });
    expect(d.spec.displayName).toBe("OpenAI Prod");
    expect(d.spec.gateways).toEqual(["gw-1"]);
    expect(d.spec).not.toHaveProperty("id");
    expect(d.spec).not.toHaveProperty("name");
  });

  it("leaves catalog membership and read-only fields out of an update", () => {
    // The edit pages spread the whole provider into the body.
    const body = { ...providerFromDocument(doc), name: "Renamed" } as UpdateLLMProviderRequest;
    const sent = JSON.parse(JSON.stringify(documentFromUpdateRequest("openai-prod", body)));
    expect(sent.metadata).toEqual({ name: "openai-prod" });
    expect(sent.spec.displayName).toBe("Renamed");
    for (const key of ["inCatalog", "uuid", "id", "name", "status", "createdAt", "updatedAt", "createdBy"]) {
      expect(sent.spec, key).not.toHaveProperty(key);
    }
    expect(sent.spec.upstream).toEqual(upstream);
  });

  it("keeps an absent gateways list apart from an empty one", () => {
    // Absent keeps the placement; [] undeploys everywhere.
    const absent = JSON.parse(JSON.stringify(documentFromUpdateRequest("p", { name: "P" })));
    const empty = JSON.parse(JSON.stringify(documentFromUpdateRequest("p", { name: "P", gateways: [] })));
    expect(absent.spec).not.toHaveProperty("gateways");
    expect(empty.spec.gateways).toEqual([]);
  });

  it("round-trips a fetched provider back to the same spec, less catalog membership", () => {
    const sent = documentFromUpdateRequest("openai-prod", providerFromDocument(doc));
    const expected: Record<string, unknown> = { ...doc.spec };
    delete expected.inCatalog;
    expect(JSON.parse(JSON.stringify(sent.spec))).toEqual(JSON.parse(JSON.stringify(expected)));
  });
});

const ORG = "/m/ai-gateway/orgs/acme%20corp";
const PROV = `${ORG}/llm-providers/open%2Fai`;
const PROXY = `${ORG}/projects/p%201/llm-proxies/px%2F1`;
const org = { orgName: "acme corp" };
const prov = { ...org, providerId: "open/ai" };
const proxy = { ...org, projName: "p 1", proxyId: "px/1" };
const dep = { ...prov, deploymentId: "d 1" };
const httpMocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

describeHttpCases("LLM provider template API", httpMocks, [
  {
    name: "listLLMProviderTemplates with paging",
    call: () => listLLMProviderTemplates(org, { limit: 5, offset: 10 }, getToken),
    method: "GET", url: `${ORG}/llm-provider-templates`, searchParams: { limit: "5", offset: "10" },
  },
  {
    name: "listLLMProviderTemplates without paging",
    call: () => listLLMProviderTemplates(org, undefined, getToken),
    method: "GET", url: `${ORG}/llm-provider-templates`,
  },
  {
    name: "createLLMProviderTemplate",
    call: () => createLLMProviderTemplate(org, { id: "t" } as never, getToken),
    method: "POST", url: `${ORG}/llm-provider-templates`, body: { id: "t" },
  },
  {
    name: "getLLMProviderTemplate",
    call: () => getLLMProviderTemplate({ ...org, templateId: "t/1" }, getToken),
    method: "GET", url: `${ORG}/llm-provider-templates/t%2F1`,
  },
  {
    name: "updateLLMProviderTemplate",
    call: () => updateLLMProviderTemplate({ ...org, templateId: "t/1" }, { name: "T" } as never, getToken),
    method: "PUT", url: `${ORG}/llm-provider-templates/t%2F1`, body: { name: "T" },
  },
  {
    name: "deleteLLMProviderTemplate",
    call: () => deleteLLMProviderTemplate({ ...org, templateId: "t/1" }, getToken),
    method: "DELETE", url: `${ORG}/llm-provider-templates/t%2F1`, returnsBody: false,
  },
  {
    name: "listLLMProviders without paging",
    call: () => listLLMProviders(org, undefined, getToken),
    method: "GET", url: BASE,
  },
]);

describeHttpCases("LLM policy availability API", httpMocks, [
  {
    name: "listAvailableLLMPolicies scoped to a provider",
    call: () => listAvailableLLMPolicies(org, { providerId: "openai" }, getToken),
    method: "GET", url: `${ORG}/llm-providers/policies`, searchParams: { providerId: "openai" },
    checksOk: false,
  },
  {
    name: "listAvailableLLMPolicies for the whole org",
    call: () => listAvailableLLMPolicies(org, undefined, getToken),
    method: "GET", url: `${ORG}/llm-providers/policies`, checksOk: false,
  },
]);

describeHttpCases("LLM proxy API", httpMocks, [
  {
    name: "listLLMProviderProxies",
    call: () => listLLMProviderProxies(prov, { limit: 1, offset: 2 }, getToken),
    method: "GET", url: `${PROV}/llm-proxies`, searchParams: { limit: "1", offset: "2" },
  },
  {
    name: "listLLMProviderProxies without paging",
    call: () => listLLMProviderProxies(prov, undefined, getToken),
    method: "GET", url: `${PROV}/llm-proxies`,
  },
  {
    name: "listLLMProxies",
    call: () => listLLMProxies({ ...org, projName: "p 1" }, { limit: 50 }, getToken),
    method: "GET", url: `${ORG}/projects/p%201/llm-proxies`, searchParams: { limit: "50" },
  },
  {
    name: "listLLMProxies without paging",
    call: () => listLLMProxies({ ...org, projName: "p 1" }, undefined, getToken),
    method: "GET", url: `${ORG}/projects/p%201/llm-proxies`,
  },
  {
    name: "createLLMProxy",
    call: () => createLLMProxy({ ...org, projName: "p 1" }, { name: "px" } as never, getToken),
    method: "POST", url: `${ORG}/projects/p%201/llm-proxies`, body: { name: "px" },
  },
  { name: "getLLMProxy", call: () => getLLMProxy(proxy, getToken), method: "GET", url: PROXY },
  {
    name: "updateLLMProxy",
    call: () => updateLLMProxy(proxy, { name: "px2" } as never, getToken),
    method: "PUT", url: PROXY, body: { name: "px2" },
  },
  {
    name: "deleteLLMProxy", call: () => deleteLLMProxy(proxy, getToken),
    method: "DELETE", url: PROXY, returnsBody: false,
  },
]);

describeHttpCases("LLM deployment API", httpMocks, [
  {
    name: "listLLMDeployments filtered",
    call: () => listLLMDeployments(prov, { gatewayId: "gw", status: "DEPLOYED" }, getToken),
    method: "GET", url: `${PROV}/deployments`, searchParams: { gatewayId: "gw", status: "DEPLOYED" },
  },
  {
    name: "listLLMDeployments unfiltered",
    call: () => listLLMDeployments(prov, undefined, getToken),
    method: "GET", url: `${PROV}/deployments`,
  },
  {
    name: "createLLMDeployment",
    call: () => createLLMDeployment(prov, { gatewayId: "gw" } as never, getToken),
    method: "POST", url: `${PROV}/deployments`, body: { gatewayId: "gw" },
  },
  {
    name: "undeployLLMProvider",
    call: () => undeployLLMProvider(prov, { deploymentId: "d1", gatewayId: "gw" }, getToken),
    method: "POST", url: `${PROV}/deployments/undeploy`, body: {},
    searchParams: { deploymentId: "d1", gatewayId: "gw" }, returnsBody: false,
  },
  {
    name: "restoreLLMDeployment",
    call: () => restoreLLMDeployment(prov, { deploymentId: "d1", gatewayId: "gw" }, getToken),
    method: "POST", url: `${PROV}/deployments/restore`, body: {},
    searchParams: { deploymentId: "d1", gatewayId: "gw" },
  },
  {
    name: "getLLMDeployment", call: () => getLLMDeployment(dep, getToken),
    method: "GET", url: `${PROV}/deployments/d%201`,
  },
  {
    name: "deleteLLMDeployment", call: () => deleteLLMDeployment(dep, getToken),
    method: "DELETE", url: `${PROV}/deployments/d%201`, returnsBody: false,
  },
  {
    name: "listLLMProviderConsumers", call: () => listLLMProviderConsumers(prov, getToken),
    method: "GET", url: `${PROV}/consumers`,
  },
]);

describeHttpCases("LLM API key API", httpMocks, [
  {
    name: "listLLMProviderAPIKeys", call: () => listLLMProviderAPIKeys(prov, getToken),
    method: "GET", url: `${PROV}/api-keys`,
  },
  {
    name: "createLLMProviderAPIKey",
    call: () => createLLMProviderAPIKey(prov, { name: "k" } as never, getToken),
    method: "POST", url: `${PROV}/api-keys`, body: { name: "k" },
  },
  {
    name: "rotateLLMProviderAPIKey",
    call: () => rotateLLMProviderAPIKey({ ...prov, keyName: "k 1" }, {} as never, getToken),
    method: "PUT", url: `${PROV}/api-keys/k%201`, body: {},
  },
  {
    name: "revokeLLMProviderAPIKey",
    call: () => revokeLLMProviderAPIKey({ ...prov, keyName: "k 1" }, getToken),
    method: "DELETE", url: `${PROV}/api-keys/k%201`, returnsBody: false,
  },
  {
    name: "listLLMProxyAPIKeys", call: () => listLLMProxyAPIKeys(proxy, getToken),
    method: "GET", url: `${PROXY}/api-keys`,
  },
  {
    name: "createLLMProxyAPIKey",
    call: () => createLLMProxyAPIKey(proxy, { name: "k" } as never, getToken),
    method: "POST", url: `${PROXY}/api-keys`, body: { name: "k" },
  },
  {
    name: "rotateLLMProxyAPIKey",
    call: () => rotateLLMProxyAPIKey({ ...proxy, keyName: "k 1" }, {} as never, getToken),
    method: "PUT", url: `${PROXY}/api-keys/k%201`, body: {},
  },
  {
    name: "revokeLLMProxyAPIKey",
    call: () => revokeLLMProxyAPIKey({ ...proxy, keyName: "k 1" }, getToken),
    method: "DELETE", url: `${PROXY}/api-keys/k%201`, returnsBody: false,
  },
]);
