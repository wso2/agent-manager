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

import { Chip, Stack, Tooltip } from "@wso2/oxygen-ui";
import type {
  AgentRegistryDeploymentStatus,
  AgentRegistryEnvironment,
} from "@agent-management-platform/types";

const STATUS_LABELS: Record<AgentRegistryDeploymentStatus, string> = {
  "not-deployed": "Not deployed",
  "in-progress": "Deploying",
  active: "Active",
  failed: "Failed",
  suspended: "Suspended",
};

const STATUS_COLORS: Record<
  AgentRegistryDeploymentStatus,
  "default" | "success" | "warning" | "error"
> = {
  "not-deployed": "default",
  "in-progress": "warning",
  active: "success",
  failed: "error",
  suspended: "default",
};

/** One chip per environment the agent has been deployed to, coloured by its status there. */
export function EnvironmentStatusChips({
  environments,
}: {
  environments: AgentRegistryEnvironment[];
}) {
  const deployed = environments.filter((env) => env.deploymentStatus !== "not-deployed");
  if (deployed.length === 0) {
    return (
      <Chip size="small" variant="outlined" color="default" label="Not deployed" />
    );
  }
  return (
    <Stack direction="row" spacing={0.5}>
      {deployed.map((env) => (
        <Tooltip key={env.name} title={`${env.displayName}: ${STATUS_LABELS[env.deploymentStatus]}`}>
          <Chip
            size="small"
            variant="outlined"
            color={STATUS_COLORS[env.deploymentStatus]}
            label={env.displayName}
          />
        </Tooltip>
      ))}
    </Stack>
  );
}
