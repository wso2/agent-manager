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

import {
    OverviewPanels,
    type AgentOverviewPanelProps,
    type DeploymentStatus,
} from "@agent-management-platform/shared-component";
import { PLUGIN_MOUNT_POINTS } from "@agent-management-platform/types";
import { Divider } from "@wso2/oxygen-ui";

interface EnvironmentSectionsContentProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    external?: boolean;
    deploymentStatus?: DeploymentStatus;
}

/**
 * An environment's sections on the agent overview. Each comes from the module
 * that owns its data (endpoints: agent-runtime; agent card and LLM/MCP
 * configs: ai-gateway; roles and groups: agent-identity; monitors and
 * observability: ai-observability), as panels at agentOverviewEnvironment.
 */
export function EnvironmentSectionsContent({
    orgId, projectId, agentId, envId, external, deploymentStatus,
}: EnvironmentSectionsContentProps) {
    return (
        <>
            <Divider sx={{ my: 1.5, mt: 0 }} />
            <OverviewPanels<AgentOverviewPanelProps>
                mountPoint={PLUGIN_MOUNT_POINTS.agentOverviewEnvironment}
                props={{ orgId, projectId, agentId, envId, external, deploymentStatus }}
            />
        </>
    );
}
