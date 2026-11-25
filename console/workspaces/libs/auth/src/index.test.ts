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

import { beforeAll, describe, expect, it, vi } from "vitest";

// Neither is under test here, and both pull in packages that are slow (or,
// for the form's design system, impossible) to load in this environment.
vi.mock("./file/LoginForm/LoginForm", () => ({ LoginForm: () => null }));
vi.mock("@thunderid/react", () => ({
  ThunderIDProvider: () => null,
  useThunderID: () => ({}),
  useUser: () => ({}),
}));

async function load(config: Record<string, unknown>) {
  vi.resetModules();
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    authConfig: {},
    ...config,
  };
  const [index, file, fileHooks, none, noneHooks, thunder, thunderHooks] = await Promise.all([
    import("./index"),
    import("./file/FileAuthContext"),
    import("./file/hooks/authHooks"),
    import("./no-auth/AuthProvider"),
    import("./no-auth/hooks/authHooks"),
    import("./thunderid/AuthProvider"),
    import("./thunderid/hooks/authHooks"),
  ]);
  const impl = {
    file: { provider: file.AuthProvider, hooks: fileHooks.useAuthHooks },
    none: { provider: none.AuthProvider, hooks: noneHooks.useAuthHooks },
    thunder: { provider: thunder.AuthProvider, hooks: thunderHooks.useAuthHooks },
  };
  return { index, impl };
}

// Each test re-imports the module graph after vi.resetModules, which is slow
// under coverage on a busy machine; the default 5s per test is too tight.
vi.setConfig({ testTimeout: 30_000 });

// The first import transforms the whole module graph, which can outlast the
// default per-test timeout. Pay that once here.
beforeAll(async () => {
  await load({});
}, 60_000);

describe("auth mode selection", () => {
  it.each([
    ["file", { authMode: "file" }, "file"],
    ["none", { authMode: "none" }, "none"],
    ["thunder", { authMode: "thunder" }, "thunder"],
    ["the service's 'idp' spelling", { authMode: "idp" }, "thunder"],
    ["nothing configured", {}, "thunder"],
    ["an unknown mode", { authMode: "saml" }, "thunder"],
    ["legacy disableAuth without a mode", { disableAuth: true }, "none"],
    ["an explicit mode over disableAuth", { authMode: "file", disableAuth: true }, "file"],
    ["an unknown mode with disableAuth", { authMode: "saml", disableAuth: true }, "none"],
  ] as const)("selects %s", async (_name, config, expected) => {
    const { index, impl } = await load(config);

    expect(index.authMode).toBe(expected);
    expect(index.isFileAuthMode).toBe(expected === "file");
    expect(index.AuthProvider).toBe(impl[expected].provider);
    expect(index.useAuthHooks).toBe(impl[expected].hooks);
  });

  it("re-exports the login form and backend session helpers", async () => {
    const { index } = await load({});
    expect(index.LoginForm).toBeTypeOf("function");
    expect(index.backendFetch).toBeTypeOf("function");
    expect(index.resolveAuthConfig).toBeTypeOf("function");
  });
});
