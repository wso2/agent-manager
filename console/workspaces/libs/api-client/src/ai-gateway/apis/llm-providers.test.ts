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
import {
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
