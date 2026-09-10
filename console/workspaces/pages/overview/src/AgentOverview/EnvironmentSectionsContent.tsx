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
    isAgentIdentityEnabled,
} from "@agent-management-platform/types";
import type { DeploymentStatus } from "@agent-management-platform/shared-component";
import { EnvAgentInterfaceCard } from "./EnvAgentInterfaceCard";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";
import { EnvCapabilitiesSection } from "./EnvCapabilitiesSection";
import { EnvConfigsSection } from "./EnvConfigsSection";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";
import { Divider, Grid } from "@wso2/oxygen-ui";

interface EnvironmentSectionsContentProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    external?: boolean;
    deploymentStatus?: DeploymentStatus;
}

/**
 * Capabilities / Agent Identity / Agent Interface / Agent Performance / Recent
 * Traces sections rendered as an EnvironmentCard's bottomContent, shared by
 * InternalAgentOverview and ExternalAgentOverview. EnvironmentCard renders
 * bottomContent unconditionally, and each section here decides for itself
 * whether it has anything to show. Agent ID and Agent Interface share one row
 * since both are compact per-environment identity/interface summaries.
 */
export function EnvironmentSectionsContent({
    orgId, projectId, agentId, envId, external, deploymentStatus,
}: EnvironmentSectionsContentProps) {
    const agentIdEnabled = isAgentIdentityEnabled();

    return (
        <>
            <Divider sx={{ my: 1.5, mt: 0 }} />
            <EnvCapabilitiesSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                external={external}
                deploymentStatus={deploymentStatus}
            />
            <Grid container spacing={2} sx={{ mb: 1.5 }}>
                {/* EnvAgentInterfaceCard renders nothing for external agents
                    (they aren't deployed through this platform), so Agent ID
                    takes the full row instead of leaving an empty half. */}
                {agentIdEnabled && (
                    <Grid size={{ xs: 12, md: external ? 12 : 6 }}>
                        <EnvAgentRolesGroupsSection
                            orgId={orgId}
                            projectId={projectId}
                            agentId={agentId}
                            envId={envId}
                        />
                    </Grid>
                )}
                <Grid size={{ xs: 12, md: agentIdEnabled ? 6 : 12 }}>
                    <EnvAgentInterfaceCard
                        orgId={orgId}
                        projectId={projectId}
                        agentId={agentId}
                        envId={envId}
                        external={external}
                        deploymentStatus={deploymentStatus}
                    />
                </Grid>
            </Grid>
            <EnvConfigsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            {/* Monitors/Observability below still use a plain loading Skeleton
                rather than CollapsibleSection, deliberately — their skeletons
                are already sized close to the real content (metric tiles,
                per-card skeletons), so there's no mismatched-height jump to
                fix for them. */}
            <EnvMonitorsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            <EnvObservabilitySection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                external={external}
            />
        </>
    );
}
