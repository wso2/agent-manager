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
vi.mock("../apis/llm-providers", async (importOriginal) => ({
  // The document conversions stay real; only the requests are mocked.
  ...(await importOriginal<object>()),
  createLLMDeployment: vi.fn(),
  createLLMProvider: vi.fn(),
  createLLMProviderAPIKey: vi.fn(),
  createLLMProviderTemplate: vi.fn(),
  createLLMProxy: vi.fn(),
  createLLMProxyAPIKey: vi.fn(),
  deleteLLMDeployment: vi.fn(),
  deleteLLMProvider: vi.fn(),
  deleteLLMProviderTemplate: vi.fn(),
  deleteLLMProxy: vi.fn(),
  getLLMDeployment: vi.fn(),
  getLLMProvider: vi.fn(),
  getLLMProviderTemplate: vi.fn(),
  getLLMProxy: vi.fn(),
  listLLMDeployments: vi.fn(),
  listLLMProviderAPIKeys: vi.fn(),
  listLLMProviderConsumers: vi.fn(),
  listLLMProviderProxies: vi.fn(),
  listLLMProviders: vi.fn(),
  listLLMProviderTemplates: vi.fn(),
  listLLMProxies: vi.fn(),
  listLLMProxyAPIKeys: vi.fn(),
  restoreLLMDeployment: vi.fn(),
  revokeLLMProviderAPIKey: vi.fn(),
  revokeLLMProxyAPIKey: vi.fn(),
  rotateLLMProviderAPIKey: vi.fn(),
  rotateLLMProxyAPIKey: vi.fn(),
  undeployLLMProvider: vi.fn(),
  updateLLMProvider: vi.fn(),
  updateLLMProviderTemplate: vi.fn(),
  updateLLMProxy: vi.fn(),
}));

import * as api from "../apis/llm-providers";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import { cleanupHooks, runMutation } from "../../test-utils/query-hooks";
import {
  useCreateLLMDeployment,
  useCreateLLMProvider,
  useCreateLLMProviderAPIKey,
  useCreateLLMProviderTemplate,
  useCreateLLMProxy,
  useCreateLLMProxyAPIKey,
  useDeleteLLMDeployment,
  useDeleteLLMProvider,
  useDeleteLLMProviderTemplate,
  useDeleteLLMProxy,
  useGetLLMDeployment,
  useGetLLMProvider,
  useGetLLMProviderTemplate,
  useGetLLMProxy,
  useListLLMDeployments,
  useListLLMProviderAPIKeys,
  useListLLMProviderConsumers,
  useListLLMProviderProxies,
  useListLLMProviders,
  useListLLMProviderTemplates,
  useListLLMProxies,
  useListLLMProxyAPIKeys,
  useRestoreLLMDeployment,
  useRevokeLLMProviderAPIKey,
  useRevokeLLMProxyAPIKey,
  useRotateLLMProviderAPIKey,
  useRotateLLMProxyAPIKey,
  useUndeployLLMProvider,
  useUpdateLLMProvider,
  useUpdateLLMProviderTemplate,
  useUpdateLLMProxy,
} from "./llm-providers";

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("ai-gateway");
  else modules.installed.clear();
};

const m = vi.mocked(api);
const org = { orgName: "o" };
const prov = { orgName: "o", providerId: "openai" };
const proj = { orgName: "o", projName: "p" };
const proxy = { ...proj, proxyId: "px" };
const dep = { ...prov, deploymentId: "d1" };
const tpl = { orgName: "o", templateId: "t1" };
const page = { limit: 10, offset: 0 };
const doc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "LlmProvider",
  metadata: { name: "openai", uid: "u-1", creationTimestamp: "2026-11-01T00:00:00Z" },
  spec: { displayName: "OpenAI", version: "v1", context: "/openai", gateways: ["gw"] },
};
const flat = api.providerFromDocument(doc as never);

describeQueryHooks("LLM provider queries", [
  {
    name: "useListLLMProviderTemplates",
    use: () => useListLLMProviderTemplates(org, page),
    api: m.listLLMProviderTemplates, args: [org, page], key: ["llm-provider-templates", org, page],
    idle: [() => useListLLMProviderTemplates({ orgName: "" })],
  },
  {
    name: "useGetLLMProviderTemplate",
    use: () => useGetLLMProviderTemplate(tpl),
    api: m.getLLMProviderTemplate, args: [tpl], key: ["llm-provider-template", tpl],
    idle: [() => useGetLLMProviderTemplate({ ...tpl, templateId: "" })],
  },
  {
    name: "useListLLMProviders flattens the documents",
    use: () => useListLLMProviders(org, page),
    api: m.listLLMProviders, args: [org, page], key: ["llm-providers", org, page],
    data: { list: [doc], pagination: { total: 9, limit: 10, offset: 0 } },
    expected: { providers: [flat], total: 9, limit: 10, offset: 0 },
    idle: [() => useListLLMProviders({ orgName: "" })],
  },
  {
    name: "useGetLLMProvider flattens the document",
    use: () => useGetLLMProvider(prov),
    api: m.getLLMProvider, args: [prov], key: ["llm-provider", prov], data: doc, expected: flat,
    idle: [() => useGetLLMProvider({ ...prov, providerId: "" })],
  },
  {
    name: "useListLLMProviderProxies",
    use: () => useListLLMProviderProxies(prov),
    api: m.listLLMProviderProxies, args: [prov, undefined], key: ["llm-provider-proxies", prov, undefined],
    idle: [() => useListLLMProviderProxies({ ...prov, providerId: "" })],
  },
  {
    name: "useListLLMProxies",
    use: () => useListLLMProxies(proj, page),
    api: m.listLLMProxies, args: [proj, page], key: ["llm-proxies", proj, page],
    idle: [() => useListLLMProxies({ ...proj, projName: "" })],
  },
  {
    name: "useGetLLMProxy",
    use: () => useGetLLMProxy(proxy),
    api: m.getLLMProxy, args: [proxy], key: ["llm-proxy", proxy],
    idle: [() => useGetLLMProxy({ ...proxy, proxyId: "" })],
  },
  {
    name: "useListLLMDeployments",
    use: () => useListLLMDeployments(prov, { status: "DEPLOYED" }),
    api: m.listLLMDeployments, args: [prov, { status: "DEPLOYED" }], key: ["llm-deployments", prov, { status: "DEPLOYED" }],
    data: [{ status: "DEPLOYED" }],
    idle: [() => useListLLMDeployments({ ...prov, providerId: "" })],
  },
  {
    name: "useGetLLMDeployment",
    use: () => useGetLLMDeployment(dep),
    api: m.getLLMDeployment, args: [dep], key: ["llm-deployment", dep],
    idle: [() => useGetLLMDeployment({ ...dep, deploymentId: "" })],
  },
  {
    name: "useListLLMProviderConsumers",
    use: () => useListLLMProviderConsumers(prov),
    api: m.listLLMProviderConsumers, args: [prov], key: ["llm-provider-consumers", prov],
    idle: [() => useListLLMProviderConsumers({ ...prov, orgName: "" })],
  },
  {
    name: "useListLLMProviderAPIKeys",
    use: () => useListLLMProviderAPIKeys(prov),
    api: m.listLLMProviderAPIKeys, args: [prov], key: ["llm-provider-api-keys", "o", "openai"],
    idle: [() => useListLLMProviderAPIKeys({ ...prov, providerId: "" })],
  },
  {
    name: "useListLLMProxyAPIKeys",
    use: () => useListLLMProxyAPIKeys(proxy),
    api: m.listLLMProxyAPIKeys, args: [proxy], key: ["llm-proxy-api-keys", "o", "p", "px"],
    idle: [() => useListLLMProxyAPIKeys({ ...proxy, proxyId: "" })],
  },
], { setModulePresent });

const SEEDED = {
  templates: ["llm-provider-templates", org],
  template: ["llm-provider-template", tpl],
  providers: ["llm-providers", org],
  provider: ["llm-provider", prov],
  catalog: ["catalog", "llm-providers"],
  providerProxies: ["llm-provider-proxies", prov],
  proxies: ["llm-proxies", proj],
  proxy: ["llm-proxy", proxy],
  deployments: ["llm-deployments", prov],
  consumers: ["llm-provider-consumers", prov],
  providerKeys: ["llm-provider-api-keys", "o", "openai"],
  proxyKeys: ["llm-proxy-api-keys", "o", "p", "px"],
} as const;

const createBody = { id: "openai", name: "OpenAI", version: "v1", context: "/openai" };
const updateBody = { id: "openai", name: "OpenAI 2" };
const keyBody = { name: "k" };
const depQuery = { deploymentId: "d1", gatewayId: "gw" };

describeMutationHooks("LLM provider mutations", SEEDED, [
  {
    name: "useCreateLLMProviderTemplate", use: useCreateLLMProviderTemplate, api: m.createLLMProviderTemplate,
    variables: { params: org, body: { id: "t" } }, args: [org, { id: "t" }], invalidates: ["templates"],
  },
  {
    name: "useUpdateLLMProviderTemplate", use: useUpdateLLMProviderTemplate, api: m.updateLLMProviderTemplate,
    variables: { params: tpl, body: { name: "T" } }, args: [tpl, { name: "T" }], invalidates: ["templates", "template"],
  },
  {
    name: "useDeleteLLMProviderTemplate", use: useDeleteLLMProviderTemplate, api: m.deleteLLMProviderTemplate,
    variables: tpl, args: [tpl], invalidates: ["templates"],
  },
  {
    name: "useCreateLLMProvider posts a document and returns the flat provider",
    use: useCreateLLMProvider, api: m.createLLMProvider,
    variables: { params: org, body: createBody },
    args: [org, api.documentFromCreateRequest(createBody as never)],
    response: doc, expected: flat, invalidates: ["providers", "catalog"],
  },
  {
    name: "useUpdateLLMProvider puts a document named by the handle",
    use: useUpdateLLMProvider, api: m.updateLLMProvider,
    variables: { params: { ...prov, providerId: "u-1" }, body: updateBody },
    args: [{ ...prov, providerId: "u-1" }, api.documentFromUpdateRequest("openai", updateBody as never)],
    response: doc, expected: flat, invalidates: ["providers", "provider", "deployments"],
  },
  {
    name: "useDeleteLLMProvider", use: useDeleteLLMProvider, api: m.deleteLLMProvider,
    variables: prov, args: [prov], invalidates: ["providers"],
  },
  {
    name: "useCreateLLMProxy", use: useCreateLLMProxy, api: m.createLLMProxy,
    variables: { params: proj, body: { name: "px" } }, args: [proj, { name: "px" }], invalidates: ["proxies", "providerProxies"],
  },
  {
    name: "useUpdateLLMProxy", use: useUpdateLLMProxy, api: m.updateLLMProxy,
    variables: { params: proxy, body: { name: "px" } }, args: [proxy, { name: "px" }],
    invalidates: ["proxies", "proxy", "providerProxies"],
  },
  {
    name: "useDeleteLLMProxy", use: useDeleteLLMProxy, api: m.deleteLLMProxy,
    variables: proxy, args: [proxy], invalidates: ["proxies", "providerProxies"],
  },
  {
    name: "useCreateLLMDeployment", use: useCreateLLMDeployment, api: m.createLLMDeployment,
    variables: { params: prov, body: { gatewayId: "gw" } }, args: [prov, { gatewayId: "gw" }], invalidates: ["deployments"],
  },
  {
    name: "useUndeployLLMProvider", use: useUndeployLLMProvider, api: m.undeployLLMProvider,
    variables: { params: prov, query: depQuery }, args: [prov, depQuery], invalidates: ["deployments", "provider"],
  },
  {
    name: "useRestoreLLMDeployment", use: useRestoreLLMDeployment, api: m.restoreLLMDeployment,
    variables: { params: prov, query: depQuery }, args: [prov, depQuery], invalidates: ["deployments", "provider"],
  },
  {
    name: "useDeleteLLMDeployment", use: useDeleteLLMDeployment, api: m.deleteLLMDeployment,
    variables: dep, args: [dep], invalidates: ["deployments"],
  },
  {
    name: "useCreateLLMProviderAPIKey", use: useCreateLLMProviderAPIKey, api: m.createLLMProviderAPIKey,
    variables: { params: prov, body: keyBody }, args: [prov, keyBody], invalidates: ["provider", "providerKeys"],
  },
  {
    name: "useRotateLLMProviderAPIKey", use: useRotateLLMProviderAPIKey, api: m.rotateLLMProviderAPIKey,
    variables: { params: { ...prov, keyName: "k" }, body: {} }, args: [{ ...prov, keyName: "k" }, {}], invalidates: ["providerKeys"],
  },
  {
    name: "useRevokeLLMProviderAPIKey", use: useRevokeLLMProviderAPIKey, api: m.revokeLLMProviderAPIKey,
    variables: { ...prov, keyName: "k" }, args: [{ ...prov, keyName: "k" }], invalidates: ["provider", "providerKeys"],
  },
  {
    name: "useCreateLLMProxyAPIKey", use: useCreateLLMProxyAPIKey, api: m.createLLMProxyAPIKey,
    variables: { params: proxy, body: keyBody }, args: [proxy, keyBody], invalidates: ["proxy", "proxyKeys"],
  },
  {
    name: "useRotateLLMProxyAPIKey", use: useRotateLLMProxyAPIKey, api: m.rotateLLMProxyAPIKey,
    variables: { params: { ...proxy, keyName: "k" }, body: {} }, args: [{ ...proxy, keyName: "k" }, {}], invalidates: ["proxyKeys"],
  },
  {
    name: "useRevokeLLMProxyAPIKey", use: useRevokeLLMProxyAPIKey, api: m.revokeLLMProxyAPIKey,
    variables: { ...proxy, keyName: "k" }, args: [{ ...proxy, keyName: "k" }], invalidates: ["proxy", "proxyKeys"],
  },
]);

describe("useUpdateLLMProvider without a handle", () => {
  afterEach(() => cleanupHooks());

  it("refuses before sending anything, since metadata.name must be the handle", async () => {
    m.updateLLMProvider.mockReset();
    const { error, invalidated } = await runMutation(
      useUpdateLLMProvider, { params: prov, body: { name: "No handle" } }, SEEDED,
    );
    expect((error as Error).message).toMatch(/handle is required/);
    expect(m.updateLLMProvider).not.toHaveBeenCalled();
    expect(invalidated).toEqual([]);
  });
});
