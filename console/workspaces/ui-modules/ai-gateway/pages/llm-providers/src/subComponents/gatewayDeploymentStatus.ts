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

import type {
  GatewayDeploymentStatus,
  LLMDeploymentResponse,
  LLMDeploymentStatus,
} from "@agent-management-platform/types";

/** How a gateway that isn't serving the provider yet reads to the user. */
export const PENDING_STATUS_CHIPS: Partial<
  Record<LLMDeploymentStatus, { label: string; color: "info" | "warning" }>
> = {
  DEPLOYING: { label: "Deploying", color: "info" },
  UNDEPLOYING: { label: "Undeploying", color: "info" },
  FAILED: { label: "Failed", color: "warning" },
};

/** The chip for a gateway that isn't serving yet; a failed undeploy reads "Undeploy failed". */
export function pendingStatusChip(
  status: GatewayDeploymentStatus,
): { label: string; color: "info" | "warning" } | undefined {
  if (status.status === "FAILED" && status.statusDesired === "UNDEPLOYED") {
    return { label: "Undeploy failed", color: "warning" };
  }
  return PENDING_STATUS_CHIPS[status.status];
}

/**
 * The status of the deployment each gateway currently serves, keyed by gateway
 * UUID. A gateway keeps its earlier deployments as ARCHIVED entries, so the
 * current one is the single non-ARCHIVED entry, whatever its age; a gateway
 * with only ARCHIVED entries has no current deployment and is left out.
 */
export function currentGatewayStatuses(
  deployments: LLMDeploymentResponse[] | undefined,
): Map<string, GatewayDeploymentStatus> {
  const statuses = new Map<string, GatewayDeploymentStatus>();
  deployments?.forEach(({ gatewayId, status, statusReason, statusDesired }) => {
    if (!gatewayId || status === "ARCHIVED") return;
    statuses.set(gatewayId, { status, statusReason, statusDesired });
  });
  return statuses;
}

/**
 * currentGatewayStatuses, plus DEPLOYING for each gateway in the provider's
 * requested set that has no current deployment yet: it was just saved.
 */
export function requestedGatewayStatuses(
  deployments: LLMDeploymentResponse[] | undefined,
  requestedGatewayIds: string[],
): Map<string, GatewayDeploymentStatus> {
  const statuses = currentGatewayStatuses(deployments);
  requestedGatewayIds.forEach((gatewayId) => {
    if (!statuses.has(gatewayId)) {
      statuses.set(gatewayId, { status: "DEPLOYING" });
    }
  });
  return statuses;
}
