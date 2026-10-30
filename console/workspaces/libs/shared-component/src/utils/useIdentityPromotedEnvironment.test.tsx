/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetProject: vi.fn(),
  useListDeploymentPipelines: vi.fn(),
  useListEnvironments: vi.fn(),
}));

import {
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { useIdentityPromotedEnvironment } from "./useIdentityPromotedEnvironment";

const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

const DEV = { name: "development", displayName: "Development" };
const STAGING = { name: "staging", displayName: "Staging" };
const PROD = { name: "production", displayName: "Production" };

const mockPipeline = (listOrder = [DEV, STAGING, PROD], isLoading = false, isError = false) => {
  vi.mocked(useListEnvironments).mockReturnValue({
    data: listOrder, isLoading, isError,
  } as unknown as ReturnType<typeof useListEnvironments>);
  vi.mocked(useGetProject).mockReturnValue({
    data: { deploymentPipeline: "default" }, isLoading: false, isError: false,
  } as unknown as ReturnType<typeof useGetProject>);
  vi.mocked(useListDeploymentPipelines).mockReturnValue({
    data: {
      deploymentPipelines: [{
        name: "default",
        promotionPaths: [
          { sourceEnvironmentRef: "development", targetEnvironmentRefs: [{ name: "staging" }] },
          { sourceEnvironmentRef: "staging", targetEnvironmentRefs: [{ name: "production" }] },
        ],
      }],
    },
    isLoading: false,
    isError: false,
  } as unknown as ReturnType<typeof useListDeploymentPipelines>);
};

const run = (envId: string) =>
  renderHook(() => useIdentityPromotedEnvironment({ orgId: "org", projectId: "proj", envId })).result.current;

beforeEach(() => {
  vi.clearAllMocks();
  mockPipeline();
});

afterEach(() => {
  delete config.featureFlags;
});

describe("useIdentityPromotedEnvironment", () => {
  describe("with the promotion UI off", () => {
    it("is never a promoted environment and gives the lookups no ids, so nothing is requested", () => {
      config.featureFlags = { enableAgentIdentity: true };
      const result = run("staging");
      expect(result.enabled).toBe(false);
      expect(result.isPromotedEnvironment).toBe(false);
      expect(result.isLoading).toBe(false);
      expect(useListEnvironments).toHaveBeenCalledWith({ orgName: undefined });
      expect(useGetProject).toHaveBeenCalledWith({ orgName: undefined, projName: undefined });
      expect(useListDeploymentPipelines).toHaveBeenCalledWith({ orgName: undefined });
    });
  });

  describe("with the promotion UI on", () => {
    beforeEach(() => {
      config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    });

    it("looks the pipeline up for the project", () => {
      run("staging");
      expect(useGetProject).toHaveBeenCalledWith({ orgName: "org", projName: "proj" });
    });

    it.each(["staging", "production"])("treats %s, after the first environment, as promoted", (envId) => {
      const result = run(envId);
      expect(result.isPromotedEnvironment).toBe(true);
      expect(result.homeEnvironment).toBe("development");
      expect(result.homeEnvironmentLabel).toBe("Development");
    });

    it("does not treat the first environment as promoted", () => {
      expect(run("development").isPromotedEnvironment).toBe(false);
    });

    it("takes the first environment from the pipeline, not from the order of the environment list", () => {
      mockPipeline([PROD, STAGING, DEV]);
      expect(run("development").isPromotedEnvironment).toBe(false);
      expect(run("production").homeEnvironment).toBe("development");
    });

    it("does not treat an environment outside the pipeline as promoted", () => {
      expect(run("sandbox").isPromotedEnvironment).toBe(false);
    });

    it("gives the display name of the environment, or its name when it has none", () => {
      expect(run("staging").envLabel).toBe("Staging");
      expect(run("sandbox").envLabel).toBe("sandbox");
    });

    it("is not promoted while the pipeline loads, and reports that it is loading", () => {
      vi.mocked(useListEnvironments).mockReturnValue({
        data: undefined, isLoading: true, isError: false,
      } as unknown as ReturnType<typeof useListEnvironments>);
      const result = run("staging");
      expect(result.isLoading).toBe(true);
      expect(result.isPromotedEnvironment).toBe(false);
    });

    it("is not promoted while only the environment list has loaded, whatever its order", () => {
      mockPipeline([PROD, STAGING, DEV]);
      vi.mocked(useListDeploymentPipelines).mockReturnValue({
        data: undefined, isLoading: true, isError: false,
      } as unknown as ReturnType<typeof useListDeploymentPipelines>);
      const result = run("development");
      expect(result.isLoading).toBe(true);
      expect(result.isPromotedEnvironment).toBe(false);
      expect(run("staging").isPromotedEnvironment).toBe(false);
    });

    it("has no promoted environment when the pipeline has no promotion paths", () => {
      mockPipeline();
      vi.mocked(useListDeploymentPipelines).mockReturnValue({
        data: { deploymentPipelines: [{ name: "default", promotionPaths: [] }] },
        isLoading: false, isError: false,
      } as unknown as ReturnType<typeof useListDeploymentPipelines>);
      expect(run("staging").isPromotedEnvironment).toBe(false);
    });

    it("has no promoted environment when the project's pipeline is not found", () => {
      vi.mocked(useGetProject).mockReturnValue({
        data: { deploymentPipeline: "other" }, isLoading: false, isError: false,
      } as unknown as ReturnType<typeof useGetProject>);
      expect(run("staging").isPromotedEnvironment).toBe(false);
    });

    it("is not promoted when the pipeline cannot be read", () => {
      mockPipeline([DEV, STAGING, PROD], false, true);
      expect(run("staging").isPromotedEnvironment).toBe(false);
    });
  });
});
