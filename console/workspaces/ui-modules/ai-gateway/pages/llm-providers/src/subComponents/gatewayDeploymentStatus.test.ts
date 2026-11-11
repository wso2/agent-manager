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

import { describe, it, expect } from "vitest";
import type { LLMDeploymentResponse } from "@agent-management-platform/types";
import {
  currentGatewayStatuses,
  pendingStatusChip,
  requestedGatewayStatuses,
} from "./gatewayDeploymentStatus";

const makeDeployment = (
  overrides: Partial<LLMDeploymentResponse>,
): LLMDeploymentResponse => ({
  deploymentId: "dep-1",
  agentName: "",
  projectName: "",
  imageId: "",
  environment: "",
  gatewayId: "gw-1",
  createdAt: "2026-01-01T00:00:00Z",
  status: "DEPLOYED",
  ...overrides,
});

describe("currentGatewayStatuses", () => {
  it("leaves out a gateway whose deployments are all ARCHIVED", () => {
    const statuses = currentGatewayStatuses([
      makeDeployment({ deploymentId: "dep-1", status: "ARCHIVED" }),
      makeDeployment({ deploymentId: "dep-2", status: "ARCHIVED" }),
    ]);
    expect(statuses.has("gw-1")).toBe(false);
  });

  it("returns the gateway's current FAILED deployment even when a newer one is ARCHIVED", () => {
    const statuses = currentGatewayStatuses([
      makeDeployment({
        deploymentId: "dep-archived",
        status: "ARCHIVED",
        createdAt: "2026-02-01T00:00:00Z",
      }),
      makeDeployment({
        deploymentId: "dep-failed",
        status: "FAILED",
        statusReason: "gateway unreachable",
        createdAt: "2026-01-01T00:00:00Z",
      }),
    ]);
    expect(statuses.get("gw-1")).toEqual({
      status: "FAILED",
      statusReason: "gateway unreachable",
    });
  });

  it("returns an empty map for undefined input", () => {
    expect(currentGatewayStatuses(undefined).size).toBe(0);
  });
});

describe("requestedGatewayStatuses", () => {
  it("counts a requested gateway with no current deployment as DEPLOYING", () => {
    const statuses = requestedGatewayStatuses(
      [makeDeployment({ gatewayId: "gw-1", status: "FAILED" })],
      ["gw-1", "gw-2"],
    );
    expect(statuses.get("gw-1")?.status).toBe("FAILED");
    expect(statuses.get("gw-2")).toEqual({ status: "DEPLOYING" });
  });
});

describe("pendingStatusChip", () => {
  it("reads a failed undeploy as Undeploy failed and a failed deploy as Failed", () => {
    expect(
      pendingStatusChip({ status: "FAILED", statusDesired: "UNDEPLOYED" })?.label,
    ).toBe("Undeploy failed");
    expect(
      pendingStatusChip({ status: "FAILED", statusDesired: "DEPLOYED" })?.label,
    ).toBe("Failed");
  });

  it("carries statusDesired through currentGatewayStatuses", () => {
    const statuses = currentGatewayStatuses([
      makeDeployment({ status: "FAILED", statusDesired: "UNDEPLOYED" }),
    ]);
    expect(statuses.get("gw-1")?.statusDesired).toBe("UNDEPLOYED");
  });
});
