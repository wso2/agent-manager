/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type {
  MCPProxy,
  ProjectMCPProxy,
  ProjectMCPProxyEnvironmentSecurity,
  SecurityConfig,
} from "@agent-management-platform/types";

const useGetMCPProxy = vi.fn();
const useGetProjectMCPProxy = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useGetMCPProxy: (...args: unknown[]) => useGetMCPProxy(...args),
  useGetProjectMCPProxy: (...args: unknown[]) => useGetProjectMCPProxy(...args),
}));

const { useMCPProxySecurity } = await import("./useMCPProxySecurity");

const apiKey: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: true, key: "X-API-Key", in: "header" },
  identity: { enabled: false },
};
const identity: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: false, key: "", in: "header" },
  identity: { enabled: true },
};
const none: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: false, key: "", in: "header" },
  identity: { enabled: false },
};

function proxy(
  endpoints: { id: string; security?: SecurityConfig; envs?: string[] }[],
): MCPProxy {
  return {
    id: "proxy-1",
    name: "weather",
    version: "1.0.0",
    endpoints: endpoints.map((endpoint) => ({
      id: endpoint.id,
      security: endpoint.security,
      environments: (endpoint.envs ?? []).map((environmentUuid) => ({
        environmentUuid,
      })),
    })),
  };
}

/** Shape react-query returns in the states this hook branches on. */
function query({
  data,
  isLoading = false,
  isError = false,
}: {
  data?: MCPProxy;
  isLoading?: boolean;
  isError?: boolean;
}) {
  useGetMCPProxy.mockReturnValue({ data, isLoading, isError });
}

/** The org GET of a project proxy: projectName set, no endpoints. */
function projectProxyStub(): MCPProxy {
  return { id: "orders", name: "Orders", version: "v1", projectName: "p1" };
}

const envAPIKey: ProjectMCPProxyEnvironmentSecurity = {
  enabled: true,
  apiKey: { enabled: true, key: "X-API-Key", in: "header" },
};
const envOff: ProjectMCPProxyEnvironmentSecurity = { enabled: false };

function projectProxy(
  environments: { uuid: string; security?: ProjectMCPProxyEnvironmentSecurity }[],
): ProjectMCPProxy {
  return {
    id: "orders",
    name: "Orders",
    version: "v1",
    context: "/orders",
    projectName: "p1",
    source: { componentName: "orders", endpointName: "mcp" },
    environments: environments.map(({ uuid, security }) => ({
      environmentName: `env-${uuid}`,
      environmentUuid: uuid,
      context: "/orders",
      security,
    })),
  };
}

function projectQuery({
  data,
  isLoading = false,
  isError = false,
}: {
  data?: ProjectMCPProxy;
  isLoading?: boolean;
  isError?: boolean;
}) {
  useGetProjectMCPProxy.mockReturnValue({ data, isLoading, isError });
}

beforeEach(() => {
  useGetMCPProxy.mockReset();
  useGetProjectMCPProxy.mockReset();
  // Disabled, as it is for an org proxy.
  useGetProjectMCPProxy.mockReturnValue({ data: undefined, isLoading: false, isError: false });
});

describe("useMCPProxySecurity", () => {
  it("scopes the answer to the selected environment's endpoint", () => {
    query({
      data: proxy([
        { id: "dev", security: apiKey, envs: ["dev-uuid"] },
        { id: "prod", security: identity, envs: ["prod-uuid"] },
      ]),
    });

    const { result } = renderHook(() =>
      useMCPProxySecurity({
        orgName: "acme",
        proxyId: "proxy-1",
        environmentUuid: "prod-uuid",
      }),
    );

    expect(result.current.authenticationType).toBe("identity");
    expect(result.current.usesIdentitySecurity).toBe(true);
    expect(result.current.spec.editableKeys).toEqual(["url"]);
    expect(result.current.isResolved).toBe(true);
  });

  it("falls back to the every-endpoint rule when the environment has no uuid", () => {
    query({
      data: proxy([
        { id: "a", security: identity, envs: ["dev-uuid"] },
        { id: "b", security: apiKey, envs: ["prod-uuid"] },
      ]),
    });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }),
    );

    // Mixed security keeps the API key field available.
    expect(result.current.authenticationType).toBe("apiKey");
    expect(result.current.spec.editableKeys).toEqual(["url", "apikey"]);
  });

  it("reports none for an unsecured endpoint, with no api key and no reference rows", () => {
    query({ data: proxy([{ id: "a", security: none, envs: ["dev-uuid"] }]) });

    const { result } = renderHook(() =>
      useMCPProxySecurity({
        orgName: "acme",
        proxyId: "proxy-1",
        environmentUuid: "dev-uuid",
      }),
    );

    expect(result.current.authenticationType).toBe("");
    expect(result.current.spec.editableKeys).toEqual(["url"]);
    expect(result.current.spec.referenceRows).toEqual([]);
    expect(result.current.isResolved).toBe(true);
  });

  it("is not resolved while the fetch is in flight", () => {
    query({ isLoading: true });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }),
    );

    expect(result.current.isLoading).toBe(true);
    expect(result.current.isResolved).toBe(false);
  });

  // The regression this guards: react-query reports isLoading false once a fetch
  // errors, so a failed load looks exactly like a genuinely unsecured endpoint.
  // Callers that trusted authenticationType would hide the API key field and drop
  // the variable from the payload — issue #1597 with the polarity reversed.
  it("is not resolved when the fetch failed, even though it reports none", () => {
    query({ isError: true });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }),
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.isError).toBe(true);
    expect(result.current.isResolved).toBe(false);
    expect(result.current.authenticationType).toBe("");
  });

  it("is resolved with nothing pending when no proxy is selected", () => {
    query({ isLoading: true });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: null }),
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.isError).toBe(false);
    expect(result.current.isResolved).toBe(true);
  });

  it("is not resolved when the fetch returned no data at all", () => {
    query({ data: undefined });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }),
    );

    expect(result.current.isResolved).toBe(false);
  });

  // A proxy record that fetched successfully but has zero endpoints is not a
  // state this hook can make a security determination from — a truthy `proxy`
  // alone is not proof the answer is real.
  it("is not resolved when the proxy fetched successfully but has no endpoints", () => {
    query({ data: proxy([]) });

    const { result } = renderHook(() =>
      useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }),
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.isError).toBe(false);
    expect(result.current.isResolved).toBe(false);
    expect(result.current.authenticationType).toBe("");
  });

  it("never asks the project route about an org proxy", () => {
    query({ data: proxy([{ id: "a", security: apiKey, envs: ["dev-uuid"] }]) });

    renderHook(() => useMCPProxySecurity({ orgName: "acme", proxyId: "proxy-1" }));

    for (const [params] of useGetProjectMCPProxy.mock.calls) {
      expect(params.projName || params.proxyId).toBeFalsy();
    }
  });

  describe("a project proxy", () => {
    it("reads its security from the project route's environments", () => {
      query({ data: projectProxyStub() });
      projectQuery({ data: projectProxy([{ uuid: "dev-uuid", security: envAPIKey }]) });

      const { result } = renderHook(() =>
        useMCPProxySecurity({
          orgName: "acme",
          proxyId: "orders",
          environmentUuid: "dev-uuid",
        }),
      );

      expect(useGetProjectMCPProxy).toHaveBeenLastCalledWith({
        orgName: "acme",
        projName: "p1",
        proxyId: "orders",
      });
      expect(result.current.authenticationType).toBe("apiKey");
      expect(result.current.usesIdentitySecurity).toBe(false);
      expect(result.current.spec.editableKeys).toEqual(["url", "apikey"]);
      expect(result.current.spec.referenceRows).toEqual([]);
      expect(result.current.isResolved).toBe(true);
    });

    it("reports none for an environment with security disabled", () => {
      query({ data: projectProxyStub() });
      projectQuery({ data: projectProxy([{ uuid: "dev-uuid", security: envOff }]) });

      const { result } = renderHook(() =>
        useMCPProxySecurity({
          orgName: "acme",
          proxyId: "orders",
          environmentUuid: "dev-uuid",
        }),
      );

      expect(result.current.authenticationType).toBe("");
      expect(result.current.spec.editableKeys).toEqual(["url"]);
      expect(result.current.isResolved).toBe(true);
    });

    it("scopes the answer to the environment whose uuid matches", () => {
      query({ data: projectProxyStub() });
      projectQuery({
        data: projectProxy([
          { uuid: "dev-uuid", security: envAPIKey },
          { uuid: "prod-uuid", security: envOff },
        ]),
      });

      const { result } = renderHook(() =>
        useMCPProxySecurity({
          orgName: "acme",
          proxyId: "orders",
          environmentUuid: "prod-uuid",
        }),
      );

      expect(result.current.authenticationType).toBe("");
      expect(result.current.spec.editableKeys).toEqual(["url"]);
    });

    it("unions every environment when no environment is given", () => {
      query({ data: projectProxyStub() });
      projectQuery({
        data: projectProxy([
          { uuid: "dev-uuid", security: envOff },
          { uuid: "prod-uuid", security: envAPIKey },
        ]),
      });

      const { result } = renderHook(() =>
        useMCPProxySecurity({ orgName: "acme", proxyId: "orders" }),
      );

      expect(result.current.authenticationType).toBe("apiKey");
      expect(result.current.spec.editableKeys).toEqual(["url", "apikey"]);
      expect(result.current.isResolved).toBe(true);
    });

    it("is not resolved while it has no environments to judge from", () => {
      query({ data: projectProxyStub() });
      projectQuery({ data: projectProxy([]) });

      const { result } = renderHook(() =>
        useMCPProxySecurity({ orgName: "acme", proxyId: "orders" }),
      );

      expect(result.current.isLoading).toBe(false);
      expect(result.current.isError).toBe(false);
      expect(result.current.isResolved).toBe(false);
    });

    it("is not resolved while the project route is loading", () => {
      query({ data: projectProxyStub() });
      projectQuery({ isLoading: true });

      const { result } = renderHook(() =>
        useMCPProxySecurity({ orgName: "acme", proxyId: "orders" }),
      );

      expect(result.current.isLoading).toBe(true);
      expect(result.current.isResolved).toBe(false);
    });

    it("is not resolved when the project route failed", () => {
      query({ data: projectProxyStub() });
      projectQuery({ isError: true });

      const { result } = renderHook(() =>
        useMCPProxySecurity({ orgName: "acme", proxyId: "orders" }),
      );

      expect(result.current.isError).toBe(true);
      expect(result.current.isResolved).toBe(false);
    });
  });
});
