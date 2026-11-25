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
import { afterEach, describe, expect, it } from "vitest";
import { AuthProvider } from "../AuthProvider";
import { useAuthHooks } from "./authHooks";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | undefined;

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
});

const claimsOf = (token: string) =>
  JSON.parse(atob(token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/"))) as {
    scope: string;
  };

describe("no-auth mode", () => {
  it("renders its children with no sign-in in between", async () => {
    const container = document.createElement("div");
    root = createRoot(container);
    await act(async () =>
      root!.render(createElement(AuthProvider, null, createElement("span", null, "app"))),
    );
    expect(container.textContent).toBe("app");
  });

  it("reports a settled, signed-in demo user", () => {
    const hooks = useAuthHooks();

    expect(hooks.isAuthenticated).toBe(true);
    expect(hooks.isLoadingUserInfo).toBe(false);
    expect(hooks.isLoadingIsAuthenticated).toBe(false);
    expect(hooks.isLoadingAccessToken).toBe(false);
    expect(hooks.userInfo).toMatchObject({
      username: "john.doe",
      orgHandle: "default",
      allowedScopes: "openid email profile amp:*",
    });
  });

  it("hands out a token whose scopes cover the console's permission checks", async () => {
    const token = await useAuthHooks().getToken();
    const scopes = claimsOf(token).scope.split(" ");

    expect(scopes).toEqual(expect.arrayContaining(["agent:create", "project:read", "org:view"]));
  });

  it("treats login, logout and silent sign-in as no-ops", async () => {
    const hooks = useAuthHooks();
    await expect(hooks.login()).resolves.toBeUndefined();
    await expect(hooks.logout()).resolves.toBeUndefined();
    await expect(hooks.trySignInSilently()).resolves.toBeUndefined();
  });
});
