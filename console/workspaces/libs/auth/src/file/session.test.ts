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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const STORAGE_KEY = "amp.file-auth.session";
const NOW = 1_700_000_000_000;

async function load(apiBaseUrl?: string) {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig: {},
    authMode: "file",
    apiBaseUrl,
  };
  return import("./session");
}

const b64url = (value: unknown) =>
  btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

const jwt = (claims: Record<string, unknown>) => `h.${b64url(claims)}.sig`;

const fetchMock = vi.fn<typeof fetch>();

// Each test re-imports the module graph after vi.resetModules, which is slow
// under coverage on a busy machine; the default 5s per test is too tight.
vi.setConfig({ testTimeout: 30_000 });

// The first import transforms the module graph, which can outlast the default
// per-test timeout. Pay that once here.
beforeAll(async () => {
  await load();
}, 60_000);

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
  window.localStorage.clear();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("session storage", () => {
  it("round-trips a written session and computes expiry from the TTL", async () => {
    const { readSession, writeSession } = await load();
    const written = writeSession("tok", 3600);

    expect(written).toEqual({ accessToken: "tok", expiresAt: NOW + 3_600_000 });
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY)!)).toEqual(written);
    expect(readSession()).toEqual(written);
  });

  it("returns null when nothing is stored", async () => {
    const { readSession } = await load();
    expect(readSession()).toBeNull();
  });

  it.each([
    ["corrupt JSON", "{not json"],
    ["a stored null", "null"],
    ["a non-object", "42"],
    ["a missing token", JSON.stringify({ expiresAt: NOW + 3_600_000 })],
    ["an empty token", JSON.stringify({ accessToken: "", expiresAt: NOW + 3_600_000 })],
    ["a non-numeric expiry", JSON.stringify({ accessToken: "t", expiresAt: "soon" })],
    ["an expired session", JSON.stringify({ accessToken: "t", expiresAt: NOW - 1 })],
    [
      "a session inside the 30s skew",
      JSON.stringify({ accessToken: "t", expiresAt: NOW + 29_000 }),
    ],
  ])("treats %s as signed out and drops the entry", async (_name, raw) => {
    const { readSession } = await load();
    window.localStorage.setItem(STORAGE_KEY, raw);

    expect(readSession()).toBeNull();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("keeps a session that expires just past the skew", async () => {
    const { readSession } = await load();
    const stored = { accessToken: "t", expiresAt: NOW + 31_000 };
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
    expect(readSession()).toEqual(stored);
  });

  it("clears the stored session", async () => {
    const { clearSession, writeSession, readSession } = await load();
    writeSession("tok", 3600);
    clearSession();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(readSession()).toBeNull();
  });

  it("survives storage that throws on every access", async () => {
    const { clearSession, readSession, writeSession } = await load();
    const boom = () => {
      throw new Error("blocked");
    };
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(boom);
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(boom);
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(boom);

    expect(readSession()).toBeNull();
    expect(writeSession("tok", 60)).toEqual({ accessToken: "tok", expiresAt: NOW + 60_000 });
    expect(() => clearSession()).not.toThrow();
  });
});

describe("decodeClaims", () => {
  it("decodes a base64url JWT payload", async () => {
    const { decodeClaims } = await load();
    // "??>" encodes with "-" and "_" in base64url, and needs padding.
    const claims = { sub: "admin", note: "??>>", scope: "a b" };
    expect(decodeClaims(jwt(claims))).toEqual(claims);
  });

  it.each([
    ["no payload segment", "only-one-part"],
    ["an empty payload", "h..sig"],
    ["a payload that is not JSON", `h.${btoa("not json")}.sig`],
  ])("returns null for %s", async (_name, token) => {
    const { decodeClaims } = await load();
    expect(decodeClaims(token)).toBeNull();
  });
});

describe("userInfoFromClaims", () => {
  it("maps server claim names onto UserInfo", async () => {
    const { userInfoFromClaims } = await load();
    expect(
      userInfoFromClaims({
        sub: "admin",
        ouId: "ou-1",
        ouHandle: "default",
        scope: "amp:agent:read amp:agent:create",
      }),
    ).toEqual({
      sub: "admin",
      username: "admin",
      displayName: "admin",
      orgId: "ou-1",
      orgHandle: "default",
      orgName: "default",
      scope: "amp:agent:read amp:agent:create",
      allowedScopes: "amp:agent:read amp:agent:create",
    });
  });

  it("returns an empty object for no claims and drops non-string or empty values", async () => {
    const { userInfoFromClaims } = await load();
    expect(userInfoFromClaims(null)).toEqual({});
    const info = userInfoFromClaims({ sub: 42, ouId: "", scope: ["a"] });
    expect(info.sub).toBeUndefined();
    expect(info.orgId).toBeUndefined();
    expect(info.scope).toBeUndefined();
  });
});

describe("loginEndpoint", () => {
  it.each([
    [undefined, "/auth/login"],
    ["http://api.example.com", "http://api.example.com/auth/login"],
    ["http://api.example.com///", "http://api.example.com/auth/login"],
  ])("builds the root login URL from apiBaseUrl %s", async (base, expected) => {
    const { loginEndpoint } = await load(base);
    expect(loginEndpoint()).toBe(expected);
  });
});

describe("requestLogin", () => {
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

  it("posts the credentials as JSON and returns the token", async () => {
    const { requestLogin } = await load("http://api.example.com/");
    fetchMock.mockResolvedValue(json({ access_token: "tok", expires_in: 900 }));

    await expect(requestLogin("admin", "s3cret")).resolves.toEqual({
      accessToken: "tok",
      expiresInSeconds: 900,
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://api.example.com/auth/login");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
    expect(JSON.parse(init?.body as string)).toEqual({ username: "admin", password: "s3cret" });
  });

  it("defaults a missing expires_in to zero", async () => {
    const { requestLogin } = await load();
    fetchMock.mockResolvedValue(json({ access_token: "tok" }));
    await expect(requestLogin("a", "b")).resolves.toEqual({
      accessToken: "tok",
      expiresInSeconds: 0,
    });
  });

  it.each([
    [401, "Incorrect username or password."],
    [404, "File-based login is not enabled on this deployment."],
    [500, "The server could not issue a token. Try again, or check the service logs."],
    [503, "The server could not issue a token. Try again, or check the service logs."],
    [400, "Could not sign in."],
  ])("maps HTTP %i to a user-facing message", async (status, message) => {
    const { requestLogin } = await load();
    fetchMock.mockResolvedValue(json({ error: "x" }, status));
    await expect(requestLogin("a", "b")).rejects.toThrow(message);
  });

  it("reports an unreachable server", async () => {
    const { requestLogin } = await load();
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));
    await expect(requestLogin("a", "b")).rejects.toThrow(
      "Could not reach the server. Check that it is running.",
    );
  });

  it("rejects a success response without a token", async () => {
    const { requestLogin } = await load();
    fetchMock.mockResolvedValue(json({ expires_in: 60 }));
    await expect(requestLogin("a", "b")).rejects.toThrow(
      "The server returned no access token.",
    );
  });
});
