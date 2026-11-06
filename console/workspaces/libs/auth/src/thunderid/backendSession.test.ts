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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const CONTEXT = "/_auth/agent-management";
const API = "/_api/agent-management";

const bffAuthConfig = (backend: Record<string, unknown> = {}) => ({
  baseUrl: window.location.origin,
  clientId: "AGENT_MANAGER_CONSOLE",
  endpoints: {
    backend: {
      signIn: `/_auth/cloud/login?returnTo=${encodeURIComponent(`${CONTEXT}/signin`)}`,
      session: `${CONTEXT}/userinfo`,
      checkSession: `${CONTEXT}/checksession`,
      refresh: `${CONTEXT}/refresh`,
      signOut: `${CONTEXT}/logout`,
      proxy: API,
      token: `${CONTEXT}/token`,
      ...backend,
    },
  },
});

const browserAuthConfig = () => ({
  baseUrl: "https://idp.example.com",
  clientId: "AGENT_MANAGER_CONSOLE",
});

async function load(authConfig: Record<string, unknown>) {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig,
    disableAuth: false,
  };
  return import("./backendSession");
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const requestedWith = (call: Parameters<typeof fetch>) =>
  new Headers(call[1]?.headers).get("X-Requested-With");

describe("resolveAuthConfig", () => {
  it("returns a config without backend endpoints as it is", async () => {
    const { resolveAuthConfig } = await load(browserAuthConfig());
    const config = browserAuthConfig();
    expect(resolveAuthConfig(config)).toBe(config);
  });

  it("keeps backend endpoints that are single-slash paths", async () => {
    const { resolveAuthConfig } = await load(bffAuthConfig());
    const config = bffAuthConfig();
    expect(resolveAuthConfig(config)).toBe(config);
  });

  it.each([
    "https://evil.example.com/_auth/agent-management",
    "//evil.example.com/_auth/agent-management",
    "/\\evil.example.com/_auth/agent-management",
    "_auth/agent-management",
  ])("turns backend sign-in off for the context %s", async (context) => {
    const { resolveAuthConfig } = await load(bffAuthConfig());
    const config = bffAuthConfig({
      session: `${context}/userinfo`,
      checkSession: `${context}/checksession`,
      refresh: `${context}/refresh`,
      signOut: `${context}/logout`,
      token: `${context}/token`,
    });
    expect(resolveAuthConfig(config).endpoints?.backend).toBeUndefined();
  });

  it("turns backend sign-in off when any context endpoint leaves the origin", async () => {
    const { resolveAuthConfig } = await load(bffAuthConfig());
    const config = bffAuthConfig({ signOut: "https://evil.example.com/logout" });
    expect(resolveAuthConfig(config).endpoints?.backend).toBeUndefined();
  });

  it.each([
    "https://evil.example.com/login",
    "//evil.example.com/login",
    "/\\evil.example.com/login",
    "",
  ])("falls back to the context's signin for the sign-in URL %s", async (signIn) => {
    const { resolveAuthConfig } = await load(bffAuthConfig());
    const resolved = resolveAuthConfig(bffAuthConfig({ signIn }));
    expect(resolved.endpoints?.backend?.signIn).toBe(`${CONTEXT}/signin`);
    expect(resolved.endpoints?.backend?.session).toBe(`${CONTEXT}/userinfo`);
  });
});

describe("backendFetch", () => {
  it("sends same-origin requests with X-Requested-With and the caller's headers", async () => {
    const { backendFetch } = await load(bffAuthConfig());
    fetchMock.mockResolvedValueOnce(json({ ok: true }));

    const response = await backendFetch(`${API}/api/v1/orgs`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`${API}/api/v1/orgs`);
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe("{}");
    expect(requestedWith(fetchMock.mock.calls[0])).toBe("XMLHttpRequest");
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
  });

  it("sends requests to another host without X-Requested-With", async () => {
    const { backendFetch } = await load(bffAuthConfig());
    fetchMock.mockResolvedValueOnce(json({}));

    await backendFetch("https://obs.example.com/api/v1/traces", {
      headers: { "Content-Type": "application/json" },
    });

    expect(requestedWith(fetchMock.mock.calls[0])).toBeNull();
  });

  it("keeps the session on a 401 from another host", async () => {
    const { backendFetch, isBackendSessionEnded } = await load(bffAuthConfig());
    fetchMock.mockResolvedValueOnce(json({}, 401));

    const response = await backendFetch("https://obs.example.com/api/v1/traces");

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(isBackendSessionEnded()).toBe(false);
  });

  it("retries a same-origin 401 once while the gateway session is live", async () => {
    const { backendFetch, isBackendSessionEnded } = await load(bffAuthConfig());
    fetchMock
      .mockResolvedValueOnce(json({}, 401))
      .mockResolvedValueOnce(json({ isSignedIn: true }))
      .mockResolvedValueOnce(json({ orgs: [] }));

    const response = await backendFetch(`${API}/api/v1/orgs`);

    expect(response.status).toBe(200);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      `${API}/api/v1/orgs`,
      `${CONTEXT}/checksession`,
      `${API}/api/v1/orgs`,
    ]);
    expect(requestedWith(fetchMock.mock.calls[1])).toBe("XMLHttpRequest");
    expect(fetchMock.mock.calls[1][1]?.credentials).toBe("include");
    expect(requestedWith(fetchMock.mock.calls[2])).toBe("XMLHttpRequest");
    expect(isBackendSessionEnded()).toBe(false);
  });

  it("returns the retry's 401 without retrying again or ending the session", async () => {
    const { backendFetch, isBackendSessionEnded } = await load(bffAuthConfig());
    fetchMock
      .mockResolvedValueOnce(json({}, 401))
      .mockResolvedValueOnce(json({ isSignedIn: true }))
      .mockResolvedValueOnce(json({}, 401));

    const response = await backendFetch(`${API}/api/v1/orgs`);

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(isBackendSessionEnded()).toBe(false);
  });

  it("ends the session on a same-origin 401 once the gateway says it is signed out", async () => {
    const { backendFetch, isBackendSessionEnded, subscribeBackendSessionEnded } =
      await load(bffAuthConfig());
    const listener = vi.fn();
    subscribeBackendSessionEnded(listener);
    fetchMock
      .mockResolvedValueOnce(json({}, 401))
      .mockResolvedValueOnce(json({ isSignedIn: false }));

    const response = await backendFetch(`${API}/api/v1/orgs`);

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(isBackendSessionEnded()).toBe(true);
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("returns a same-origin 401 without resending a streamed body", async () => {
    const { backendFetch, isBackendSessionEnded } = await load(bffAuthConfig());
    fetchMock
      .mockResolvedValueOnce(json({}, 401))
      .mockResolvedValueOnce(json({ isSignedIn: true }));

    const response = await backendFetch(`${API}/api/v1/orgs`, {
      method: "POST",
      body: new ReadableStream(),
    });

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(isBackendSessionEnded()).toBe(false);
  });

  it.each([
    ["the check fails", () => Promise.reject(new TypeError("network"))],
    ["the check answers 500", () => Promise.resolve(json({}, 500))],
  ])("keeps the session and retries when %s", async (_, check) => {
    const { backendFetch, isBackendSessionEnded } = await load(bffAuthConfig());
    fetchMock
      .mockResolvedValueOnce(json({}, 401))
      .mockImplementationOnce(check)
      .mockResolvedValueOnce(json({}));

    const response = await backendFetch(`${API}/api/v1/orgs`);

    expect(response.status).toBe(200);
    expect(isBackendSessionEnded()).toBe(false);
  });

  it("leaves requests alone when the browser signs in itself", async () => {
    const { backendFetch, isBackendSessionEnded } = await load(browserAuthConfig());
    fetchMock.mockResolvedValueOnce(json({}, 401));
    const init = { headers: { Authorization: "Bearer t" } };

    const response = await backendFetch("/api/v1/orgs", init);

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][1]).toBe(init);
    expect(isBackendSessionEnded()).toBe(false);
  });
});

describe("clearBrowserSignInSession", () => {
  const own = [
    "session_data-instance_0-AGENT_MANAGER_CONSOLE",
    "temporary_data-instance_0-AGENT_MANAGER_CONSOLE",
    "hybrid_data-instance_1-AGENT_MANAGER_CONSOLE",
  ];
  const others = [
    "session_data-instance_0-WSO2_CLOUD_CONSOLE",
    "session_data-instance_0",
    "session_data-instance_x-AGENT_MANAGER_CONSOLE",
    "theme-AGENT_MANAGER_CONSOLE",
  ];

  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
    [...own, ...others].forEach((key) => {
      window.localStorage.setItem(key, "v");
      window.sessionStorage.setItem(key, "v");
    });
  });

  it("removes this console's SDK keys from both storages and keeps the rest", async () => {
    const { clearBrowserSignInSession } = await load(bffAuthConfig());

    clearBrowserSignInSession("AGENT_MANAGER_CONSOLE");

    [window.localStorage, window.sessionStorage].forEach((storage) => {
      own.forEach((key) => expect(storage.getItem(key)).toBeNull());
      others.forEach((key) => expect(storage.getItem(key)).toBe("v"));
    });
  });

  it("removes nothing without a client ID", async () => {
    const { clearBrowserSignInSession } = await load(bffAuthConfig());

    clearBrowserSignInSession(undefined);

    expect(window.localStorage.length).toBe(own.length + others.length);
    expect(window.sessionStorage.length).toBe(own.length + others.length);
  });
});
