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

import type { Environment } from "@agent-management-platform/types";
import { Box, Tooltip, Typography } from "@wso2/oxygen-ui";
import {
  DeploymentStatus,
  IsolationTierBadge,
} from "@agent-management-platform/shared-component";

interface EnvironmentSingleHeaderProps {
  environment: Environment;
  status?: DeploymentStatus;
  /**
   * Theme color path (e.g. "success.main") for the status dot — same
   * mapping EnvironmentTabsBar uses.
   */
  dotColor: string;
}

const STATUS_LABEL: Record<DeploymentStatus, string> = {
  [DeploymentStatus.ACTIVE]: "Deployed",
  [DeploymentStatus.INACTIVE]: "Not Deployed",
  [DeploymentStatus.DEPLOYING]: "Deploying",
  [DeploymentStatus.ERROR]: "Error",
  [DeploymentStatus.FAILED]: "Error",
  [DeploymentStatus.SUSPENDED]: "Suspended",
};

/**
 * EnvironmentCard's `tabsHeader` slot for the single-environment case — a tab
 * strip has nothing to switch between with only one environment, but the env's
 * sandbox level, name, and deployment state are still worth surfacing in the
 * same spot, styled like EnvironmentTabsBar's tab label (badge + name + dot).
 * The dot carries the deployment state on hover rather than a separate chip,
 * keeping the header as compact as a tab label.
 */
export function EnvironmentSingleHeader({
  environment,
  status,
  dotColor,
}: EnvironmentSingleHeaderProps) {
  return (
    <Box display="flex" alignItems="center" gap={0.75} mb={1}>
      <IsolationTierBadge tier={environment.isolationTier} size={14} />
      <Typography variant="h6">{environment.displayName ?? environment.name}</Typography>
      <Tooltip title={status ? STATUS_LABEL[status] : ""}>
        <Box
          sx={{
            width: 8,
            height: 8,
            borderRadius: "50%",
            bgcolor: dotColor,
            flexShrink: 0,
          }}
        />
      </Tooltip>
    </Box>
  );
}
