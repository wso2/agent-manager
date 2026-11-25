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

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthHooks } from "./authHooks";

// Covers the browser-mode actions (login, logout, token and silent sign-in
// fallbacks) and the access-token decode states; authHooks.test.ts covers the
// backend session.

const sdk = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  profile: null as Record<string, unknown> | null,
}));

vi.mock("@thunderid/react", () => ({
  useThunderID: () => sdk.state,
  useUser: () => ({ flattenedProfile: sdk.profile }),
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const b64url = (value: unknown) =>
  btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const jwt = (claims: Record<string, unknown>) => `${b64url({ alg: "RS256" })}.${b64url(claims)}.sig`;

const assign = vi.fn();
const originalLocation = window.location;
let root: Root | undefined;
let latest: AuthHooks | undefined;

async function mount(authConfig: Record<string, unknown> = {}, basePath = "") {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig: { baseUrl: "https://idp.example.com", clientId: "C", ...authConfig },
    disableAuth: false,
    basePath,
  };
  const { useAuthHooks } = await import("./authHooks");
  const Probe = () => {
    latest = useAuthHooks();
    return null;
  };
  root = createRoot(document.createElement("div"));
  const render = () => act(async () => root!.render(createElement(Probe)));
  await render();
  return { render };
}

const signedIn = (overrides: Record<string, unknown> = {}) => ({
  isSignedIn: true,
  isInitialized: true,
  isLoading: false,
  getAccessToken: vi.fn(() => Promise.resolve(jwt({ sub: "u", scope: "amp:agent:read" }))),
  signIn: vi.fn(),
  signOut: vi.fn(() => Promise.resolve()),
  signInSilently: vi.fn(() => Promise.resolve(true)),
  signInOptions: { resource: "urn:wso2:amp" },
  ...overrides,
});

// Each test re-imports the module graph after vi.resetModules, which is slow
// under coverage on a busy machine; the default 5s per test is too tight.
vi.setConfig({ testTimeout: 30_000 });

// The first import transforms the module graph, which can outlast the default
// per-test timeout. Pay that once here.
beforeAll(async () => {
  await import("./authHooks");
}, 60_000);

beforeEach(() => {
  latest = undefined;
  sdk.profile = null;
  assign.mockReset();
  Object.defineProperty(window, "location", {
    configurable: true,
    value: { ...originalLocation, assign },
  });
});

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
});

describe("useAuthHooks actions in the browser", () => {
  it("login passes the configured sign-in options to the SDK", async () => {
    sdk.state = signedIn();
    await mount();

    latest!.login();

    expect(sdk.state.signIn).toHaveBeenCalledWith({ resource: "urn:wso2:amp" });
  });

  it("returns the SDK's access token and silent sign-in result", async () => {
    sdk.state = signedIn();
    await mount();

    expect(await latest!.getToken()).toBe(jwt({ sub: "u", scope: "amp:agent:read" }));
    await expect(latest!.trySignInSilently()).resolves.toBe(true);
  });

  it("rejects token and silent sign-in calls the SDK does not provide", async () => {
    sdk.state = signedIn({ getAccessToken: undefined, signInSilently: undefined });
    await mount();

    await expect(latest!.getToken()).rejects.toThrow("getAccessToken is not available");
    await expect(latest!.trySignInSilently()).rejects.toThrow(
      "signInSilently is not available",
    );
  });

  it.each([
    ["the base-path login page", {}, "console", "/console/login"],
    [
      "the configured after-sign-out URL",
      { afterSignOutUrl: "https://example.com/bye" },
      "",
      "https://example.com/bye",
    ],
  ])("logout signs out and navigates to %s", async (_name, config, base, target) => {
    sdk.state = signedIn();
    await mount(config, base);

    await act(async () => latest!.logout());

    expect(sdk.state.signOut).toHaveBeenCalled();
    expect(assign).toHaveBeenCalledWith(target);
  });

  it("still navigates away when the SDK's sign-out fails", async () => {
    sdk.state = signedIn({ signOut: vi.fn(() => Promise.reject(new Error("network"))) });
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    await mount();

    await act(async () => latest!.logout());

    expect(assign).toHaveBeenCalledWith("/login");
    expect(errors).toHaveBeenCalledWith("Error during signOut:", expect.any(Error));
    errors.mockRestore();
  });
});

describe("useAuthHooks access-token claims", () => {
  it("merges token claims over the profile and reads names from the profile", async () => {
    sdk.profile = {
      attributes: { email: "a@x.io", username: "ada", given_name: "Ada", family_name: "L" },
    };
    sdk.state = signedIn();
    await mount();

    expect(latest!.isAuthenticated).toBe(true);
    expect(latest!.isLoadingAccessToken).toBe(false);
    expect(latest!.userInfo).toMatchObject({
      email: "a@x.io",
      username: "ada",
      givenName: "Ada",
      familyName: "L",
      sub: "u",
      scope: "amp:agent:read",
    });
  });

  it("stays loading while the token is still being read", async () => {
    sdk.state = signedIn({ getAccessToken: vi.fn(() => new Promise<string>(() => undefined)) });
    await mount();

    expect(latest!.isAuthenticated).toBe(true);
    expect(latest!.isLoadingAccessToken).toBe(true);
    expect(latest!.userInfo.scope).toBeUndefined();
  });

  it.each([
    ["an empty token", () => Promise.resolve("")],
    ["a token that is not a JWT", () => Promise.resolve("opaque")],
    ["a token whose payload is not JSON", () => Promise.resolve(`h.${btoa("nope")}.s`)],
    ["a rejected token read", () => Promise.reject(new Error("expired"))],
  ])("settles with no claims for %s", async (_name, getAccessToken) => {
    sdk.state = signedIn({ getAccessToken: vi.fn(getAccessToken) });
    await mount();

    expect(latest!.isLoadingAccessToken).toBe(false);
    expect(latest!.userInfo.sub).toBeUndefined();
    expect(latest!.userInfo.scope).toBeUndefined();
  });

  it("settles with no claims when the SDK has no token reader", async () => {
    sdk.state = signedIn({ getAccessToken: undefined });
    await mount();

    expect(latest!.isLoadingAccessToken).toBe(false);
    expect(latest!.userInfo.sub).toBeUndefined();
  });

  it("reports loading until the SDK has initialised", async () => {
    sdk.state = signedIn({ isInitialized: false, isLoading: true });
    await mount();

    expect(latest!.isAuthenticated).toBe(false);
    expect(latest!.isLoadingIsAuthenticated).toBe(true);
    expect(latest!.isLoadingUserInfo).toBe(true);
    expect(latest!.isLoadingAccessToken).toBe(true);
    expect(sdk.state.getAccessToken).not.toHaveBeenCalled();
  });

  it("drops the claims after signing out", async () => {
    sdk.state = signedIn();
    const { render } = await mount();
    expect(latest!.userInfo.sub).toBe("u");

    sdk.state = { ...sdk.state, isSignedIn: false };
    await render();

    expect(latest!.isAuthenticated).toBe(false);
    expect(latest!.userInfo.sub).toBeUndefined();
    expect(latest!.isLoadingAccessToken).toBe(false);
  });

  it("treats a missing SDK context as signed out", async () => {
    sdk.state = undefined as unknown as Record<string, unknown>;
    await mount();

    expect(latest!.isAuthenticated).toBe(false);
    expect(latest!.isLoadingIsAuthenticated).toBe(true);
  });
});

describe("useAuthHooks on the backend with no id_token claims", () => {
  it("settles with no claims", async () => {
    sdk.state = signedIn({
      getDecodedIdToken: vi.fn(() => Promise.resolve({})),
      getGrantedPermissions: vi.fn(() => Promise.resolve(undefined)),
    });
    await mount({
      baseUrl: window.location.origin,
      endpoints: {
        backend: {
          signIn: "/_auth/a/signin",
          session: "/_auth/a/userinfo",
          checkSession: "/_auth/a/checksession",
        },
      },
    });

    expect(await latest!.getToken()).toBe("");
    expect(latest!.isLoadingAccessToken).toBe(false);
    expect(latest!.userInfo.sub).toBeUndefined();
  });
});
