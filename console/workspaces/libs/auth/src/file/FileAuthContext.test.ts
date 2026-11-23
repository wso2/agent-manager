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

import { act, Component, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthHooks } from "../thunderid/hooks/authHooks";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const STORAGE_KEY = "amp.file-auth.session";
const NOW = 1_700_000_000_000;
const MAX_TIMEOUT_MS = 2_147_483_647;

const b64url = (value: unknown) =>
  btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const TOKEN = `h.${b64url({ sub: "admin", ouId: "ou-1", ouHandle: "default", scope: "amp:agent:read" })}.sig`;

const fetchMock = vi.fn<typeof fetch>();
const assign = vi.fn();
const originalLocation = window.location;

let root: Root | undefined;
let latest: AuthHooks | undefined;
let context: ReturnType<Modules["useFileAuth"]> | undefined;

type Modules = typeof import("./FileAuthContext") & typeof import("./hooks/authHooks");

const runtimeConfig = { authConfig: {}, authMode: "file", basePath: "" };
let mod: Modules;

// One module graph for the whole file: re-importing it per test is slow
// enough under coverage to hit the per-test timeout. globalConfig keeps a
// reference to runtimeConfig, so tests change basePath by mutating it.
async function load(): Promise<Modules> {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = runtimeConfig;
  const provider = await import("./FileAuthContext");
  const hooks = await import("./hooks/authHooks");
  return { ...provider, ...hooks };
}

async function mount(basePath = "") {
  runtimeConfig.basePath = basePath;
  // Fake the clock only once the module graph is loaded: a faked setTimeout
  // stalls the dynamic import. Switching from the Date-only fake needs a
  // reset in between, or the second install does not take.
  vi.useRealTimers();
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  const Probe = () => {
    latest = mod.useAuthHooks();
    context = mod.useFileAuth();
    return null;
  };
  root = createRoot(document.createElement("div"));
  await act(async () =>
    root!.render(createElement(mod.AuthProvider, null, createElement(Probe))),
  );
  return mod;
}

const storeSession = (expiresAt: number, accessToken = TOKEN) =>
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken, expiresAt }));

// Rendering is slow under coverage on a busy machine; the default 5s per test
// is too tight.
vi.setConfig({ testTimeout: 30_000 });

// The first import of the module graph transforms it, which can outlast the
// default per-test timeout. Pay that once here.
beforeAll(async () => {
  mod = await load();
}, 60_000);

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
  window.localStorage.clear();
  latest = undefined;
  context = undefined;
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  assign.mockReset();
  Object.defineProperty(window, "location", {
    configurable: true,
    value: { ...originalLocation, assign },
  });
});

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
});

describe("file-mode AuthProvider", () => {
  it("starts signed out with no stored session", async () => {
    await mount();

    expect(latest!.isAuthenticated).toBe(false);
    expect(latest!.isLoadingIsAuthenticated).toBe(false);
    expect(latest!.userInfo).toEqual({});
    expect(await latest!.getToken()).toBe("");
  });

  it("restores a stored session and exposes its claims and token", async () => {
    storeSession(NOW + 3_600_000);
    await mount();

    expect(latest!.isAuthenticated).toBe(true);
    expect(latest!.isLoadingUserInfo).toBe(false);
    expect(latest!.isLoadingAccessToken).toBe(false);
    expect(latest!.userInfo).toMatchObject({
      username: "admin",
      orgId: "ou-1",
      orgHandle: "default",
      scope: "amp:agent:read",
    });
    expect(await latest!.getToken()).toBe(TOKEN);
  });

  it("signs in with the form's credentials and stores the issued token", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ access_token: TOKEN, expires_in: 600 }), { status: 200 }),
    );
    await mount();

    await act(async () => context!.signIn("admin", "pw"));

    expect(JSON.parse(fetchMock.mock.calls[0][1]?.body as string)).toEqual({
      username: "admin",
      password: "pw",
    });
    expect(latest!.isAuthenticated).toBe(true);
    expect(await latest!.getToken()).toBe(TOKEN);
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY)!)).toEqual({
      accessToken: TOKEN,
      expiresAt: NOW + 600_000,
    });
  });

  it("propagates a failed sign-in and stays signed out", async () => {
    fetchMock.mockResolvedValue(new Response("{}", { status: 401 }));
    await mount();

    await act(async () => {
      await expect(context!.signIn("admin", "bad")).rejects.toThrow(
        "Incorrect username or password.",
      );
    });

    expect(latest!.isAuthenticated).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it.each([
    ["", "/login"],
    ["console", "/console/login"],
  ])("logout clears the session and navigates to login (basePath %j)", async (base, target) => {
    storeSession(NOW + 3_600_000);
    await mount(base);

    await act(async () => latest!.logout());

    expect(latest!.isAuthenticated).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(assign).toHaveBeenCalledWith(target);
  });

  it("signs the user out when the token expires", async () => {
    storeSession(NOW + 60_000);
    await mount();
    expect(latest!.isAuthenticated).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });

    expect(latest!.isAuthenticated).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("waits in bounded steps for an expiry past the setTimeout limit", async () => {
    const ttl = MAX_TIMEOUT_MS + 3_600_000;
    storeSession(NOW + ttl);
    await mount();

    await act(async () => {
      vi.advanceTimersByTime(MAX_TIMEOUT_MS);
    });
    // The first step only re-checks the clock; the session is still valid.
    expect(latest!.isAuthenticated).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(3_600_000);
    });
    expect(latest!.isAuthenticated).toBe(false);
  });

  it("file hooks never redirect or sign in silently", async () => {
    await mount();

    expect(latest!.login()).toBeUndefined();
    await expect(latest!.trySignInSilently()).resolves.toBeUndefined();
    expect(assign).not.toHaveBeenCalled();
  });
});

describe("useFileAuth", () => {
  it("throws outside the provider", async () => {
    const { useFileAuth } = mod;
    const Orphan = () => {
      useFileAuth();
      return null;
    };
    let caught: unknown;
    class Boundary extends Component<{ children: ReactNode }, { failed: boolean }> {
      state = { failed: false };
      static getDerivedStateFromError() {
        return { failed: true };
      }
      componentDidCatch(error: unknown) {
        caught = error;
      }
      render() {
        return this.state.failed ? null : this.props.children;
      }
    }
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    root = createRoot(document.createElement("div"));

    await act(async () =>
      root!.render(createElement(Boundary, null, createElement(Orphan))),
    );
    errors.mockRestore();

    expect((caught as Error | undefined)?.message).toBe(
      "useFileAuth must be used within the file-mode AuthProvider",
    );
  });
});
