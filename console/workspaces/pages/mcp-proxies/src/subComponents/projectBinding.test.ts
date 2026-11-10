/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { ProjectMCPProxyEnvironment } from "@agent-management-platform/types";
import {
  applyBindingConfigChange,
  checkBindingRequest,
  bindingEndpointConfig,
  bindingRequestFrom,
  projectProxyEndpointDraft,
  bindingRequiresAPIKey,
  bindingSecurityMethod,
  DEFAULT_API_KEY_HEADER,
  keptBindingRequest,
  planURLProxySave,
  sameOrigin,
  proxyRequestFrom,
} from "./projectBinding";

const base: ProjectMCPProxyEnvironment = {
  environmentName: "development",
  environmentUuid: "env-1",
  context: "/weather",
};

describe("bindingRequiresAPIKey", () => {
  it("defaults to an API key, as the service does", () => {
    expect(bindingRequiresAPIKey(undefined)).toBe(true);
    expect(bindingRequiresAPIKey({ enabled: true })).toBe(true);
  });

  it("is off when security or its API key is off", () => {
    expect(bindingRequiresAPIKey({ enabled: false })).toBe(false);
    expect(
      bindingRequiresAPIKey({ enabled: true, apiKey: { enabled: false } }),
    ).toBe(false);
  });
});

describe("an OAuth (identity) binding", () => {
  const identity = { enabled: true, identity: { enabled: true } };

  it("asks for no API key and reads as OAuth", () => {
    expect(bindingRequiresAPIKey(identity)).toBe(false);
    expect(bindingSecurityMethod(identity)).toBe("identity");
    expect(bindingSecurityMethod({ enabled: false, identity: { enabled: true } })).toBe("none");
    expect(bindingSecurityMethod(undefined)).toBe("apiKey");
  });

  it("keeps OAuth when any tab saves the binding", () => {
    expect(bindingRequestFrom({ ...base, security: identity }).security).toEqual(identity);
  });

  it("shows OAuth to the org tabs", () => {
    const proxy = {
      id: "weather",
      name: "Weather",
      version: "v1",
      context: "/weather",
      projectName: "p1",
      environments: [],
    };
    const config = bindingEndpointConfig(proxy, { ...base, security: identity });
    expect(config.security).toEqual(identity);
  });
});

describe("bindingRequestFrom", () => {
  it("keeps every field the binding has, so a save changes only its own", () => {
    const request = bindingRequestFrom({
      ...base,
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: {
        enabled: true,
        apiKey: { enabled: true, key: "X-Weather-Key", in: "query" },
      },
      policies: [
        { name: "basic-ratelimit", version: "v1", params: { limit: 10 } },
      ],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request).toEqual({
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: {
        enabled: true,
        apiKey: { enabled: true, key: "X-Weather-Key", in: "query" },
      },
      policies: [
        { name: "basic-ratelimit", version: "v1", params: { limit: 10 } },
      ],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
  });

  it("sends the upstream credential without its value, which keeps the stored one", () => {
    const request = bindingRequestFrom({
      ...base,
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request.upstreamAuth).toEqual({
      type: "api-key",
      header: "Authorization",
    });
    expect(request.upstreamAuth && "value" in request.upstreamAuth).toBe(false);
  });

  it("leaves out what the binding does not have", () => {
    expect(bindingRequestFrom(base)).toEqual({ policies: [] });
    expect(
      bindingRequestFrom({ ...base, upstreamAuth: { type: "none" } })
        .upstreamAuth,
    ).toBeUndefined();
  });

  it("keeps security off, and fills the default key name when on", () => {
    expect(
      bindingRequestFrom({ ...base, security: { enabled: false } }).security,
    ).toEqual({
      enabled: false,
    });
    expect(
      bindingRequestFrom({ ...base, security: { enabled: true } }).security,
    ).toEqual({
      enabled: true,
      apiKey: { enabled: true, key: DEFAULT_API_KEY_HEADER, in: "header" },
    });
  });
});

describe("proxyRequestFrom", () => {
  it("keeps what a URL proxy has, with its upstream and no source", () => {
    expect(
      proxyRequestFrom({
        id: "docs",
        name: "Docs",
        version: "v1",
        context: "/docs",
        description: "d",
        projectName: "p1",
        upstream: { url: "https://mcp.example.com/mcp" },
        environments: [],
      }),
    ).toEqual({
      name: "Docs",
      version: "v1",
      context: "/docs",
      description: "d",
      upstream: { url: "https://mcp.example.com/mcp" },
    });
  });
});

describe("binding as an org endpoint config", () => {
  const urlProxy = {
    id: "docs",
    name: "Docs",
    version: "v1",
    context: "/docs",
    projectName: "p1",
    upstream: { url: "https://mcp.example.com/mcp" },
    environments: [],
  };
  const env = {
    ...base,
    upstreamAuth: { type: "api-key" as const, header: "Authorization" },
    policies: [{ name: "basic-ratelimit", version: "v1" }],
    capabilities: { tools: [{ name: "search" }] },
    capabilitiesSource: "caller" as const,
  };

  it("shows the proxy's URL with the binding's credential, tools and policies", () => {
    const config = bindingEndpointConfig(urlProxy, env);
    expect(config.upstream?.main).toEqual({
      url: "https://mcp.example.com/mcp",
      auth: { type: "api-key", header: "Authorization" },
    });
    expect(config.capabilities).toEqual({ tools: [{ name: "search" }] });
    expect(config.policies).toEqual([
      { name: "basic-ratelimit", version: "v1" },
    ]);
  });

  it("keeps the tools when a tab changes only the policies", () => {
    const binding = applyBindingConfigChange(env, {
      policies: [],
    });
    expect(binding.policies).toEqual([]);
    expect(binding.capabilities).toEqual({ tools: [{ name: "search" }] });
    expect(binding.upstreamAuth).toEqual({
      type: "api-key",
      header: "Authorization",
    });
  });

  it("sends a new credential value, and never takes the URL from the tab", () => {
    const binding = applyBindingConfigChange(env, {
      upstream: {
        main: {
          url: "https://mcp2.example.com/mcp",
          auth: { type: "api-key", header: "X-Key", value: "s3cret" },
        },
      },
    });
    expect(binding.upstreamAuth).toEqual({
      type: "api-key",
      header: "X-Key",
      value: "s3cret",
    });
    expect(binding).not.toHaveProperty("upstream");
  });

  it("clears the credential when the tab removes it", () => {
    const binding = applyBindingConfigChange(env, {
      upstream: { main: { url: "https://elsewhere/mcp" } },
    });
    expect(binding.upstreamAuth).toBeUndefined();
  });
});

describe("binding timeouts", () => {
  const urlProxy = {
    id: "docs",
    name: "Docs",
    version: "v1",
    context: "/docs",
    projectName: "p1",
    upstream: { url: "https://mcp.example.com/mcp" },
    environments: [],
  };
  const env = { ...base, resilience: { timeout: "15s", idleTimeout: "1m" } };

  it("are kept on a save that changes something else, and shown to the tabs", () => {
    expect(bindingRequestFrom(env).resilience).toEqual({
      timeout: "15s",
      idleTimeout: "1m",
    });
    expect(bindingEndpointConfig(urlProxy, env).resilience).toEqual({
      timeout: "15s",
      idleTimeout: "1m",
    });
  });

  it("are changed or cleared by the Connection tab", () => {
    const changed = applyBindingConfigChange(env, {
      resilience: { timeout: "30s" },
    });
    expect(changed.resilience).toEqual({ timeout: "30s" });
    const cleared = applyBindingConfigChange(env, {
      resilience: undefined,
    });
    expect(cleared.resilience).toBeUndefined();
  });
});

describe("discovered capabilities", () => {
  it("are not sent back, so discovery keeps them", () => {
    const request = bindingRequestFrom({
      ...base,
      capabilities: { tools: [{ name: "search" }] },
      capabilitiesSource: "discovered",
    });
    expect(request.capabilities).toBeUndefined();
  });
});

describe("projectProxyEndpointDraft", () => {
  it("is the URL server's one endpoint, deployed to each of its environments", () => {
    const draft = projectProxyEndpointDraft({
      id: "docs",
      name: "Docs",
      version: "v1",
      context: "/docs",
      projectName: "p1",
      upstream: { url: "https://mcp.example.com/mcp" },
      environments: [
        {
          ...base,
          gatewayId: "gw-1",
          upstreamAuth: { type: "api-key", header: "Authorization" },
          capabilities: { tools: [{ name: "search" }] },
        },
        { ...base, environmentName: "production", environmentUuid: "env-2" },
      ],
    });
    expect(draft.url).toBe("https://mcp.example.com/mcp");
    expect(draft.authHeader).toBe("Authorization");
    expect(draft.authValue).toBe("");
    expect(draft.environments).toEqual(["env-1", "env-2"]);
    expect(draft.gatewayByEnv).toEqual({ "env-1": "gw-1" });
    expect(draft.fetchedInfo.tools).toEqual([{ name: "search" }]);
  });
});

describe("keptBindingRequest", () => {
  const draft = {
    id: "primary",
    name: "",
    url: "https://mcp.example.com/mcp",
    authHeader: "Authorization",
    authValue: "",
    resilienceTimeout: "15s",
    resilienceIdleTimeout: "",
    environments: ["e1", "e2"],
    fetchedInfo: {},
  };
  const other = {
    ...base,
    upstreamAuth: { type: "api-key" as const, header: "X-Other" },
    resilience: { timeout: "60s" },
  };
  const edited = {
    upstreamAuth: {
      type: "api-key" as const,
      header: "Authorization",
      value: "new",
    },
    resilience: { timeout: "30s" },
  };

  it("keeps an environment's own credential and timeouts when the form did not change them", () => {
    const request = keptBindingRequest(other, draft, draft, edited);
    expect(request.upstreamAuth).toEqual({
      type: "api-key",
      header: "X-Other",
    });
    expect(request.resilience).toEqual({ timeout: "60s" });
  });

  it("keeps an environment with no credential without one", () => {
    const request = keptBindingRequest(base, draft, draft, edited);
    expect(request.upstreamAuth).toBeUndefined();
  });

  it("applies a credential or timeouts the form changed", () => {
    const request = keptBindingRequest(
      other,
      draft,
      { ...draft, authValue: "new", resilienceTimeout: "30s" },
      edited,
    );
    expect(request.upstreamAuth).toEqual(edited.upstreamAuth);
    expect(request.resilience).toEqual({ timeout: "30s" });
  });

  it("clears them when the form removed them", () => {
    const request = keptBindingRequest(
      other,
      draft,
      { ...draft, authHeader: "", resilienceTimeout: "" },
      {},
    );
    expect(request.upstreamAuth).toBeUndefined();
    expect(request.resilience).toBeUndefined();
  });
});

describe("planURLProxySave", () => {
  const withAuth = (name: string, uuid: string, header?: string) => ({
    ...base,
    environmentName: name,
    environmentUuid: uuid,
    ...(header ? { upstreamAuth: { type: "api-key" as const, header } } : {}),
  });
  const proxy = {
    id: "docs",
    name: "Docs",
    version: "v1",
    context: "/docs",
    projectName: "p1",
    upstream: { url: "https://mcp.example.com/mcp" },
    environments: [
      withAuth("dev", "e1", "X-Key"),
      withAuth("prod", "e2", "X-Key"),
    ],
  };
  const initial = {
    id: "primary",
    name: "",
    url: "https://mcp.example.com/mcp",
    authHeader: "X-Key",
    authValue: "",
    resilienceTimeout: "",
    resilienceIdleTimeout: "",
    environments: ["e1", "e2"],
    fetchedInfo: {},
  };
  const kinds = (steps: ReturnType<typeof planURLProxySave>) =>
    steps.map((s) => (s.kind === "proxy" ? "proxy" : `${s.kind}:${s.envName}`));

  it("saves the proxy, then each environment with its own credential, on the same host", () => {
    const steps = planURLProxySave(
      proxy,
      initial,
      initial,
      {
        url: "https://mcp.example.com/v2/mcp",
        upstreamAuth: { type: "api-key", header: "X-Key" },
      },
      [],
    );
    expect(kinds(steps)).toEqual(["proxy", "binding:dev", "binding:prod"]);
    expect(
      steps[0].kind === "proxy" && steps[0].resetBindingAuthFor,
    ).toBeUndefined();
  });

  it("changes the proxy first, then removes unassigned environments", () => {
    const draft = { ...initial, environments: ["e1"] };
    const steps = planURLProxySave(
      proxy,
      initial,
      draft,
      { url: initial.url },
      [],
    );
    expect(kinds(steps)).toEqual(["proxy", "remove:prod", "binding:dev"]);
  });

  it("has the proxy update clear stored credentials on a host move, then sets the typed one", () => {
    const draft = {
      ...initial,
      url: "https://other.example.com/mcp",
      authValue: "new",
    };
    const auth = { type: "api-key" as const, header: "X-Key", value: "new" };
    const steps = planURLProxySave(
      proxy,
      initial,
      draft,
      {
        url: draft.url,
        upstreamAuth: auth,
      },
      [],
    );
    // No request clears a credential before the proxy update, which clears them itself.
    expect(kinds(steps)).toEqual(["proxy", "binding:dev", "binding:prod"]);
    const [update, setDev] = steps;
    // Only the environments this page loaded: the service refuses one it did not know of.
    expect(update.kind === "proxy" && update.resetBindingAuthFor).toEqual([
      "dev",
      "prod",
    ]);
    expect(setDev.kind === "binding" && setDev.body.upstreamAuth).toEqual(auth);
  });

  it("refuses a move to another host that would keep a stored credential, before any request", () => {
    expect(() =>
      planURLProxySave(
        proxy,
        initial,
        { ...initial, url: "https://other.example.com/mcp" },
        {
          url: "https://other.example.com/mcp",
          upstreamAuth: { type: "api-key", header: "X-Key" },
        },
        [],
      ),
    ).toThrow(/new host/);
  });

  it("moves to another host when the credential is removed", () => {
    const draft = {
      ...initial,
      url: "https://other.example.com/mcp",
      authHeader: "",
    };
    const steps = planURLProxySave(
      proxy,
      initial,
      draft,
      { url: draft.url },
      [],
    );
    expect(kinds(steps)).toEqual(["proxy", "binding:dev", "binding:prod"]);
    expect(steps[0].kind === "proxy" && steps[0].resetBindingAuthFor).toEqual([
      "dev",
      "prod",
    ]);
    expect(
      steps.every((s) => s.kind !== "binding" || !s.body.upstreamAuth),
    ).toBe(true);
  });

  it("still applies the edit on a retry after the first environment was saved", () => {
    // The first save changed dev's header and failed on prod. The baseline is the page's first
    // load, so the header is still an edit, and prod gets it too.
    const retried = {
      ...proxy,
      environments: [
        withAuth("dev", "e1", "X-New"),
        withAuth("prod", "e2", "X-Key"),
      ],
    };
    const draft = { ...initial, authHeader: "X-New" };
    const steps = planURLProxySave(
      retried,
      initial,
      draft,
      {
        url: initial.url,
        upstreamAuth: { type: "api-key", header: "X-New" },
      },
      [],
    );
    const prod = steps.find(
      (s) => s.kind === "binding" && s.envName === "prod",
    );
    expect(prod?.kind === "binding" && prod.body.upstreamAuth?.header).toBe(
      "X-New",
    );
  });
});

describe("sameOrigin", () => {
  it("compares scheme, host and port", () => {
    expect(
      sameOrigin("https://a.example/mcp", "https://a.example:443/v2"),
    ).toBe(true);
    expect(sameOrigin("https://a.example/mcp", "http://a.example/mcp")).toBe(
      false,
    );
    expect(sameOrigin("https://a.example/mcp", "not a url")).toBe(false);
  });
});

describe("a binding still reading its API key from the query", () => {
  const legacy = {
    ...base,
    security: {
      enabled: true,
      apiKey: { enabled: true, key: "api_key", in: "query" as const },
    },
  };

  it("is refused on a save from another tab, before it is sent", () => {
    const request = applyBindingConfigChange(legacy, { policies: [] });
    expect(() => checkBindingRequest(request, "dev")).toThrow(
      /dev environment.*query/,
    );
  });

  it("saves once moved to a header, or with API keys off", () => {
    expect(() =>
      checkBindingRequest(
        {
          ...bindingRequestFrom(legacy),
          security: {
            enabled: true,
            apiKey: { enabled: true, key: "X-Key", in: "header" },
          },
        },
        "dev",
      ),
    ).not.toThrow();
    expect(() =>
      checkBindingRequest(
        { ...bindingRequestFrom(legacy), security: { enabled: false } },
        "dev",
      ),
    ).not.toThrow();
  });

  it("stops a URL server's edit before the proxy is changed", () => {
    const proxy = {
      id: "docs",
      name: "Docs",
      version: "v1",
      context: "/docs",
      projectName: "p1",
      upstream: { url: "https://mcp.example.com/mcp" },
      environments: [
        { ...base, environmentName: "dev", environmentUuid: "e1" },
        { ...legacy, environmentName: "prod", environmentUuid: "e2" },
      ],
    };
    const draft = {
      id: "primary",
      name: "",
      url: "https://mcp.example.com/v2/mcp",
      authHeader: "",
      authValue: "",
      resilienceTimeout: "",
      resilienceIdleTimeout: "",
      environments: ["e1", "e2"],
      fetchedInfo: {},
    };
    expect(() =>
      planURLProxySave(
        proxy,
        { ...draft, url: proxy.upstream.url },
        draft,
        { url: draft.url },
        [],
      ),
    ).toThrow(/prod environment/);
  });
});
