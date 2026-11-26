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

import { createElement, act } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({
  getPricing: vi.fn(async () => ({
    updatedAt: null,
    providers: [],
    total: 0,
    limit: 50,
    offset: 0,
    models: [],
  })),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
  useIsUIModuleInstalled: () => true,
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({
    getToken: async () => "t",
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {},
  markSessionExpired: vi.fn(),
  useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/pricing", () => api);

import { usePricing } from "./pricing";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

async function render<T>(useHook: () => T) {
  const result = {} as { current: T };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(
      createElement(
        QueryClientProvider,
        { client: new QueryClient() },
        createElement(Probe),
      ),
    ),
  );
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return result;
}

beforeEach(() => api.getPricing.mockClear());

describe("usePricing", () => {
  it("reads the catalog with the query and the token getter", async () => {
    const query = { search: "gpt", provider: "openai", limit: 50, offset: 100 };
    await render(() => usePricing({ orgName: "acme" }, query));
    expect(api.getPricing).toHaveBeenCalledWith(
      { orgName: "acme" },
      query,
      expect.any(Function),
    );
  });

  it("waits for an organization", async () => {
    await render(() => usePricing({ orgName: "" }));
    expect(api.getPricing).not.toHaveBeenCalled();
  });
});
