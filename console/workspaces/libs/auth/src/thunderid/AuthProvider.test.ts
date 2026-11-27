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

const sdk = vi.hoisted(() => ({
  providerProps: undefined as Record<string, unknown> | undefined,
  // The SDK's temporary data as the provider first saw it.
  temporaryDataAtStart: undefined as string | null | undefined,
  getAccessToken: undefined as unknown as () => Promise<string>,
}));

vi.mock("@thunderid/react", async () => {
  const { createElement: h, Fragment } = await import("react");
  return {
    ThunderIDProvider: ({ children, ...props }: { children?: unknown; clientId?: string }) => {
      sdk.providerProps = props;
      if (sdk.temporaryDataAtStart === undefined) {
        sdk.temporaryDataAtStart = window.localStorage.getItem(
          `temporary_data-instance_0-${props.clientId}`,
        );
      }
      return h(Fragment, null, children as never);
    },
    useThunderID: () => ({ getAccessToken: sdk.getAccessToken }),
  };
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const CLIENT_ID = "AGENT_MANAGER_CONSOLE";
const CONTEXT = "/_auth/agent-management";
const SDK_KEY = `session_data-instance_0-${CLIENT_ID}`;

const bffAuthConfig = {
  baseUrl: window.location.origin,
  clientId: CLIENT_ID,
  endpoints: {
    backend: {
      // Not a local path: the provider must be handed the rewritten one.
      signIn: "https://elsewhere.example.com/signin",
      session: `${CONTEXT}/userinfo`,
      checkSession: `${CONTEXT}/checksession`,
      token: `${CONTEXT}/token`,
      proxy: "/_api/agent-management",
    },
  },
};

const browserAuthConfig = { baseUrl: "https://idp.example.com", clientId: CLIENT_ID };

const fetchMock = vi.fn<typeof fetch>();
let root: Root | undefined;
let container: HTMLDivElement;

async function mount(authConfig: Record<string, unknown>) {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig,
    disableAuth: false,
  };
  const { AuthProvider } = await import("./AuthProvider");
  const session = await import("./backendSession");
  container = document.createElement("div");
  root = createRoot(container);
  await act(async () =>
    root!.render(createElement(AuthProvider, null, createElement("span", null, "app"))),
  );
  return session;
}

const sentAuthorization = () =>
  new Headers(fetchMock.mock.lastCall?.[1]?.headers).get("Authorization");

// Each test re-imports the module graph after vi.resetModules, which is slow
// under coverage on a busy machine; the default 5s per test is too tight.
vi.setConfig({ testTimeout: 30_000 });

// The first import transforms the module graph, which can outlast the default
// per-test timeout. Pay that once here.
beforeAll(async () => {
  await import("./AuthProvider");
}, 60_000);

beforeEach(() => {
  sdk.providerProps = undefined;
  sdk.temporaryDataAtStart = undefined;
  sdk.getAccessToken = vi.fn(() => Promise.resolve("sdk-token"));
  window.localStorage.clear();
  window.sessionStorage.clear();
  fetchMock.mockReset();
  fetchMock.mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  vi.unstubAllGlobals();
});

describe("ThunderID AuthProvider on the backend", () => {
  it("hands the SDK the resolved config and renders the app", async () => {
    await mount(bffAuthConfig);

    expect(container.textContent).toBe("app");
    const backend = (sdk.providerProps?.endpoints as { backend: Record<string, string> })
      .backend;
    expect(backend.signIn).toBe(`${CONTEXT}/signin`);
    expect(sdk.providerProps?.clientId).toBe(CLIENT_ID);
  });

  it("clears a leftover browser sign-in session for its own client only", async () => {
    window.localStorage.setItem(SDK_KEY, "{}");
    window.sessionStorage.setItem(SDK_KEY, "{}");
    window.localStorage.setItem("session_data-instance_0-OTHER_CLIENT", "{}");
    window.localStorage.setItem("unrelated", "1");

    await mount(bffAuthConfig);

    expect(window.localStorage.getItem(SDK_KEY)).toBeNull();
    expect(window.sessionStorage.getItem(SDK_KEY)).toBeNull();
    expect(window.localStorage.getItem("session_data-instance_0-OTHER_CLIENT")).toBe("{}");
    expect(window.localStorage.getItem("unrelated")).toBe("1");
  });

  it("lends the SDK's access token to backend token requests while mounted", async () => {
    const session = await mount(bffAuthConfig);

    await session.fetchWithBackendToken("/api/thing");
    expect(sentAuthorization()).toBe("Bearer sdk-token");

    await act(async () => root!.unmount());
    root = undefined;

    await session.fetchWithBackendToken("/api/thing");
    expect(sentAuthorization()).toBeNull();
  });
});

describe("ThunderID AuthProvider in the browser", () => {
  it("passes the config through and leaves storage and tokens alone", async () => {
    window.localStorage.setItem(SDK_KEY, "{}");
    const session = await mount(browserAuthConfig);

    expect(sdk.providerProps).toEqual(browserAuthConfig);
    expect(window.localStorage.getItem(SDK_KEY)).toBe("{}");

    await session.fetchWithBackendToken("/api/thing");
    expect(sentAuthorization()).toBeNull();
  });

  it("drops the previous page's refresh timer before the SDK starts", async () => {
    const tempKey = `temporary_data-instance_0-${CLIENT_ID}`;
    window.localStorage.setItem(
      tempKey,
      JSON.stringify({ op_config_initiated: true, refresh_token_timer: "7" }),
    );

    await mount(browserAuthConfig);

    expect(JSON.parse(sdk.temporaryDataAtStart!)).toEqual({ op_config_initiated: true });
  });
});
