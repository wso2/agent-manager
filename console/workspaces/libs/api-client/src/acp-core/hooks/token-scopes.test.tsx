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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Types from "@agent-management-platform/types";
import { renderHook, type RenderedHook } from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({
  auth: {} as { userInfo?: { scope?: string }; isLoadingAccessToken?: boolean },
  config: { disableAuth: false },
}));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ui.auth }));
vi.mock("@agent-management-platform/types", async (importOriginal) => ({
  ...(await importOriginal<typeof Types>()),
  globalConfig: ui.config,
}));

import { useTokenScopes } from "./token-scopes";

let rendered: RenderedHook<unknown> | undefined;
afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  ui.auth = {};
  ui.config.disableAuth = false;
});

async function scopes() {
  const hook = await renderHook(useTokenScopes);
  rendered = hook as RenderedHook<unknown>;
  return hook.result.current;
}

describe("useTokenScopes", () => {
  it("splits the token's scope claim into a set", async () => {
    ui.auth = { userInfo: { scope: "openid  amp:events:read amp:events:write" }, isLoadingAccessToken: false };
    const result = await scopes();
    expect([...result.scopes]).toEqual(["openid", "amp:events:read", "amp:events:write"]);
    expect(result).toMatchObject({ enforced: true, resolved: true });
  });

  it("is unresolved and empty while the token is still being read", async () => {
    ui.auth = { isLoadingAccessToken: true };
    const result = await scopes();
    expect(result.resolved).toBe(false);
    expect(result.scopes.size).toBe(0);
  });

  it("enforces nothing when auth is disabled", async () => {
    ui.config.disableAuth = true;
    expect((await scopes()).enforced).toBe(false);
  });
});
