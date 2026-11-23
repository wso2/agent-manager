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

import { describe, expect, it } from "vitest";
import {
  buildCreateLLMProviderRequest,
  mapLLMProviderTemplatesToCards,
  toProviderId,
} from "./llmProviderPayload";
import { addLLMProviderSchema, editLLMProviderSchema } from "./schema";
import type { AddLLMProviderFormValues, TemplateCard } from "./AddLLMProviderForm";
import type { LLMProviderTemplateResponse } from "@agent-management-platform/types";

const baseValues: AddLLMProviderFormValues = {
  templateId: "openai",
  displayName: " Prod OpenAI ",
  version: " v1.0 ",
  description: "",
  context: "",
  upstreamUrl: " https://api.openai.com/v1 ",
  apiKey: "",
  resilienceTimeout: "",
  resilienceIdleTimeout: "",
  gatewayIds: [],
};

const bearerTemplate: TemplateCard = {
  id: "openai",
  handle: "openai-handle",
  name: "OpenAI",
  authType: "api-key",
  authHeader: "X-Key",
  authValuePrefix: "Bearer ",
};

describe("toProviderId", () => {
  it.each([
    ["My Provider", "my-provider"],
    ["  --Weird__Name!!  ", "weird-name"],
    ["!!!", ""],
  ])("%j -> %j", (input, expected) => {
    expect(toProviderId(input)).toBe(expected);
  });
});

describe("mapLLMProviderTemplatesToCards", () => {
  it("maps template metadata onto cards", () => {
    const cards = mapLLMProviderTemplatesToCards([
      {
        id: "openai",
        name: "OpenAI",
        description: "desc",
        metadata: {
          logoUrl: "logo.png",
          endpointUrl: "https://api",
          auth: { type: "api-key", header: "Authorization", valuePrefix: "Bearer " },
        },
      } as unknown as LLMProviderTemplateResponse,
      { id: "bare", name: "Bare" } as unknown as LLMProviderTemplateResponse,
    ]);
    expect(cards).toEqual([
      {
        id: "openai",
        handle: "openai",
        name: "OpenAI",
        description: "desc",
        image: "logo.png",
        hasTemplateUrl: true,
        endpointUrl: "https://api",
        hasTemplateAuthType: true,
        hasTemplateAuthHeader: true,
        authType: "api-key",
        authHeader: "Authorization",
        authValuePrefix: "Bearer ",
      },
      expect.objectContaining({
        id: "bare",
        hasTemplateUrl: false,
        hasTemplateAuthType: false,
        hasTemplateAuthHeader: false,
        image: undefined,
      }),
    ]);
  });

  it("returns an empty list when there are no templates", () => {
    expect(mapLLMProviderTemplatesToCards(undefined)).toEqual([]);
  });
});

describe("buildCreateLLMProviderRequest", () => {
  it("builds a minimal request without auth, security, resilience or policies", () => {
    expect(buildCreateLLMProviderRequest(baseValues, [], [bearerTemplate])).toEqual({
      id: "prod-openai",
      name: "Prod OpenAI",
      version: "v1.0",
      context: "",
      template: "openai-handle",
      upstream: { main: { url: "https://api.openai.com/v1", auth: undefined } },
      resilience: undefined,
      description: undefined,
      security: undefined,
      policies: undefined,
      gateways: undefined,
      accessControl: { exceptions: [], mode: "allow_all" },
    });
  });

  it("prefixes the API key, enables key security and carries guardrails, timeouts and gateways", () => {
    const req = buildCreateLLMProviderRequest(
      {
        ...baseValues,
        apiKey: " sk-1 ",
        context: " /prod ",
        description: " main ",
        resilienceTimeout: "5s",
        gatewayIds: ["gw-1"],
      },
      [
        { name: "pii", version: "v1", settings: { mask: true } },
        { name: "len", version: "v2" },
      ] as never,
      [bearerTemplate],
    );
    expect(req.upstream.main?.auth).toEqual({ type: "api-key", header: "X-Key", value: "Bearer sk-1" });
    expect(req.security).toEqual({ enabled: true, apiKey: { enabled: true, in: "header" } });
    expect(req.resilience).toEqual({ timeout: "5s", idleTimeout: undefined });
    expect(req.context).toBe("/prod");
    expect(req.description).toBe("main");
    expect(req.gateways).toEqual(["gw-1"]);
    expect(req.policies).toEqual([
      { name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mask: true } }] },
      { name: "len", version: "v2", paths: [{ path: "/*", methods: ["*"], params: {} }] },
    ]);
  });

  it("falls back to defaults when the template is unknown or the name is unusable", () => {
    const req = buildCreateLLMProviderRequest(
      { ...baseValues, displayName: "!!!", apiKey: "raw", resilienceIdleTimeout: "1m" },
      [],
      [],
    );
    expect(req.id).toBe("llm-provider");
    expect(req.name).toBe("!!!");
    expect(req.template).toBe("openai");
    expect(req.upstream.main?.auth).toEqual({ type: "api-key", header: "Authorization", value: "raw" });
    expect(req.resilience).toEqual({ timeout: undefined, idleTimeout: "1m" });
  });

  it("uses the template name when it has no handle", () => {
    const req = buildCreateLLMProviderRequest(baseValues, [], [
      { ...bearerTemplate, handle: "" },
    ]);
    expect(req.template).toBe("OpenAI");
  });
});

describe("addLLMProviderSchema", () => {
  const valid = { ...baseValues, displayName: "Prod", version: "v1.0", upstreamUrl: "https://x.io" };
  const firstError = (values: Partial<AddLLMProviderFormValues>) => {
    const res = addLLMProviderSchema.safeParse({ ...valid, ...values });
    return res.success ? undefined : res.error.issues[0].message;
  };

  it("accepts a valid form", () => {
    expect(firstError({})).toBeUndefined();
    expect(firstError({ context: "/a/b", resilienceTimeout: "500ms", resilienceIdleTimeout: "1.5h" })).toBeUndefined();
  });

  it.each([
    [{ templateId: " " }, "Select a provider template"],
    [{ displayName: "" }, "Display name is required"],
    [{ displayName: "x".repeat(121) }, "Display name must be at most 120 characters"],
    [{ version: "" }, "Version is required"],
    [{ version: "1" }, "Version must match v<major>.<minor> (e.g., v1.0)"],
    [{ description: "x".repeat(513) }, "Description cannot exceed 512 characters"],
    [{ context: "/" }, expect.stringContaining("root path (/) is invalid")],
    [{ context: "no-slash" }, expect.stringContaining("Context must start with /")],
    [{ upstreamUrl: "not a url" }, "Enter a valid URL"],
    [{ resilienceTimeout: "5" }, "Enter a duration like 5s, 500ms, or 1m"],
    [{ resilienceIdleTimeout: "soon" }, "Enter a duration like 5s, 500ms, or 1m"],
  ])("rejects %j", (values, message) => {
    expect(firstError(values as Partial<AddLLMProviderFormValues>)).toEqual(message);
  });
});

describe("editLLMProviderSchema", () => {
  it.each([
    [{ name: "ok" }, true],
    [{ name: "" }, false],
    [{ name: "a" }, false],
    [{ name: "ok", description: "x".repeat(513) }, false],
  ])("%j valid=%s", (values, ok) => {
    expect(editLLMProviderSchema.safeParse(values).success).toBe(ok);
  });
});
