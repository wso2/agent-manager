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

import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  FeatureManagementProvider,
  useFeatureManagement,
  type FeatureManagementProviderProps,
} from "./FeatureManagementProvider";
import { useEnvironmentFeatures, useGatewayFeatures, usePipelineFeatures } from "./features";
import { useFeatureFlag, useResourceAccess } from "./hooks";

const withProvider = (props: Omit<FeatureManagementProviderProps, "children">) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return <FeatureManagementProvider {...props}>{children}</FeatureManagementProvider>;
  };

describe("FeatureManagementProvider", () => {
  it("enables everything without a provider", () => {
    const { result } = renderHook(() => ({
      env: useEnvironmentFeatures(),
      agent: useResourceAccess("agent"),
      loading: useFeatureManagement().isAccessLoading,
    }));
    expect(result.current).toEqual({
      env: { canView: true, canWrite: true },
      agent: { canRead: true, canWrite: true },
      loading: false,
    });
  });

  it("hides only the externally managed feature", () => {
    const { result } = renderHook(
      () => ({
        env: useEnvironmentFeatures(),
        gateway: useGatewayFeatures(),
        pipeline: usePipelineFeatures(),
      }),
      { wrapper: withProvider({ featureFlags: { externallyManagedGateway: true } }) },
    );
    expect(result.current.gateway).toEqual({ canView: false, canWrite: false });
    expect(result.current.env).toEqual({ canView: true, canWrite: true });
    expect(result.current.pipeline).toEqual({ canView: true, canWrite: true });
  });

  it("keeps read and write access separate", () => {
    const { result } = renderHook(() => useEnvironmentFeatures(), {
      wrapper: withProvider({ featureAccess: { env_write: false } }),
    });
    expect(result.current).toEqual({ canView: true, canWrite: false });
  });

  it("denies write on a managed resource the user cannot read", () => {
    const { result } = renderHook(() => usePipelineFeatures(), {
      wrapper: withProvider({ featureAccess: { pipeline_read: false } }),
    });
    expect(result.current).toEqual({ canView: false, canWrite: false });
  });

  it("reads flags, defaulting to off", () => {
    const { result } = renderHook(() => ({
      selfHosted: useFeatureFlag("enableSelfHostedGateways"),
      agentId: useFeatureFlag("enableAgentIdentity"),
    }), {
      wrapper: withProvider({ featureFlags: { enableSelfHostedGateways: true } }),
    });
    expect(result.current).toEqual({ selfHosted: true, agentId: false });
  });

  it("reads a resource's access pair", () => {
    const { result } = renderHook(() => useResourceAccess("llm_provider"), {
      wrapper: withProvider({ featureAccess: { llm_provider_write: false } }),
    });
    expect(result.current).toEqual({ canRead: true, canWrite: false });
  });

  it("lets a nested provider override only the keys it sets", () => {
    const Outer = withProvider({
      featureAccess: { env_read: false, env_write: false },
      isAccessLoading: true,
    });
    const Inner = withProvider({ featureAccess: { env_read: true } });
    const { result } = renderHook(() => useFeatureManagement(), {
      wrapper: ({ children }) => (
        <Outer>
          <Inner>{children}</Inner>
        </Outer>
      ),
    });
    expect(result.current.hasAccess("env_read")).toBe(true);
    expect(result.current.hasAccess("env_write")).toBe(false);
    expect(result.current.isAccessLoading).toBe(true);
  });
});
