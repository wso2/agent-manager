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

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthHooks } from "./authHooks";

const sdk = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
}));

vi.mock("@thunderid/react", () => ({
  useThunderID: () => sdk.state,
  useUser: () => ({ flattenedProfile: null }),
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const CONTEXT = "/_auth/agent-management";

const bffAuthConfig = {
  baseUrl: window.location.origin,
  clientId: "AGENT_MANAGER_CONSOLE",
  endpoints: {
    backend: {
      signIn: `${CONTEXT}/signin`,
      session: `${CONTEXT}/userinfo`,
      checkSession: `${CONTEXT}/checksession`,
      refresh: `${CONTEXT}/refresh`,
      signOut: `${CONTEXT}/logout`,
      proxy: "/_api/agent-management",
      token: `${CONTEXT}/token`,
    },
  },
};

const browserAuthConfig = {
  baseUrl: "https://idp.example.com",
  clientId: "AGENT_MANAGER_CONSOLE",
};

let root: Root | undefined;
let latest: AuthHooks | undefined;

async function mount(authConfig: Record<string, unknown>) {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig,
    disableAuth: false,
  };
  const { useAuthHooks } = await import("./authHooks");
  const session = await import("../backendSession");
  const Probe = () => {
    latest = useAuthHooks();
    return null;
  };
  const container = document.createElement("div");
  root = createRoot(container);
  const render = () => act(async () => root!.render(createElement(Probe)));
  await render();
  return { render, session };
}

const signedIn = (overrides: Record<string, unknown> = {}) => ({
  isSignedIn: true,
  isInitialized: true,
  isLoading: false,
  getAccessToken: vi.fn(() => Promise.resolve("header.eyJzdWIiOiJ1In0.sig")),
  getDecodedIdToken: vi.fn(() => Promise.resolve({ sub: "u", email: "u@example.com" })),
  getGrantedPermissions: vi.fn(() => Promise.resolve(["amp:agent:read"])),
  signIn: vi.fn(),
  signOut: vi.fn(),
  ...overrides,
});

beforeEach(() => {
  latest = undefined;
});

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
});

describe("useAuthHooks on the backend", () => {
  it("never asks the SDK for an access token", async () => {
    sdk.state = signedIn();
    await mount(bffAuthConfig);

    expect(await latest!.getToken()).toBe("");
    expect(sdk.state.getAccessToken).not.toHaveBeenCalled();
    expect(latest!.userInfo.sub).toBe("u");
    expect(latest!.userInfo.scope).toBe("amp:agent:read");
  });

  it("ends the session when the SDK turns signed out and keeps it ended", async () => {
    sdk.state = signedIn();
    const { render, session } = await mount(bffAuthConfig);
    expect(latest!.isAuthenticated).toBe(true);

    sdk.state = { ...sdk.state, isSignedIn: false };
    await render();

    expect(session.isBackendSessionEnded()).toBe(true);
    expect(latest!.isAuthenticated).toBe(false);

    sdk.state = { ...sdk.state, isSignedIn: true };
    await render();

    expect(latest!.isAuthenticated).toBe(false);
    expect(latest!.userInfo.sub).toBeUndefined();
    expect(latest!.isLoadingAccessToken).toBe(false);
  });

  it("does not end a session that was never signed in", async () => {
    sdk.state = signedIn({ isSignedIn: false });
    const { session } = await mount(bffAuthConfig);

    expect(session.isBackendSessionEnded()).toBe(false);
    expect(latest!.isAuthenticated).toBe(false);
  });

  it("reads as signed out once a request ends the session", async () => {
    sdk.state = signedIn();
    const { session } = await mount(bffAuthConfig);
    expect(latest!.isAuthenticated).toBe(true);

    await act(async () => session.endBackendSession());

    expect(latest!.isAuthenticated).toBe(false);
  });
});

describe("useAuthHooks in the browser", () => {
  it("reads the access token and leaves sign-out to the SDK", async () => {
    sdk.state = signedIn();
    const { render, session } = await mount(browserAuthConfig);

    expect(sdk.state.getAccessToken).toHaveBeenCalled();
    expect(latest!.userInfo.sub).toBe("u");

    sdk.state = { ...sdk.state, isSignedIn: false };
    await render();

    expect(session.isBackendSessionEnded()).toBe(false);
    expect(latest!.isAuthenticated).toBe(false);
  });
});
