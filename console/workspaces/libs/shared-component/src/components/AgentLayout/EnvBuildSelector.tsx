/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { Box, Button, ButtonGroup } from "@wso2/oxygen-ui";
import { generatePath, Link, useParams } from "react-router-dom";
import { TabStatus, TopNavBarGroup } from "../LinkTab";
import { useGetAgent, useListAgentDeployments, useListEnvironments } from "@agent-management-platform/api-client";
import { useEffect, useMemo } from "react";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { Wrench as BuildOutlined } from "@wso2/oxygen-ui-icons-react";


export const EnvBuildSelector: React.FC = () => {
    const { orgId, agentId, projectId, envId } = useParams();
    const { data: agent } = useGetAgent({
        orgName: orgId ?? 'default',
        projName: projectId ?? 'default',
        agentName: agentId ?? ''
    });
    const { data: environments } = useListEnvironments({ orgName: orgId ?? 'default' });
    const { data: deployments } = useListAgentDeployments({
        orgName: orgId || '',
        projName: projectId || '',
        agentName: agentId || '',
    }, {
        enabled: agent?.provisioning.type === 'internal',
    });

    const sortedEnvironments = useMemo(() =>
        environments?.sort((a) => a.isProduction ? 1 : -1) ?? [], [environments]);
    // Set first environment as default if no environment is selected
    useEffect(() => {

    }, [sortedEnvironments]);

    if (agent?.provisioning.type === 'external' || !agent) {
        return null;
    }

    return (
        <Box display="flex" gap={1}>
            <ButtonGroup
                variant="text"
                color="inherit"
                orientation="horizontal"
                size="small"
                aria-label="vertical outlined button group"
            >
                <Button
                    component={Link}
                    to={generatePath(absoluteRouteMap.
                        children.org.children.projects.children.agents.
                        path,
                        {
                            orgId: orgId ?? 'default',
                            projectId: projectId ?? 'default',
                            agentId: agentId ?? '',
                        })}
                    variant={envId ? "text" : "contained"}
                    size="small"
                    color="primary"
                    startIcon={<BuildOutlined />}
                >
                    Build
                </Button>
            </ButtonGroup>
            <TopNavBarGroup
                tabs={sortedEnvironments.map((env) => {
                    return {
                        to: generatePath(absoluteRouteMap.
                            children.org.children.projects.children.agents.
                            children.environment.path,
                            {
                                orgId: orgId ?? 'default',
                                projectId: projectId ?? 'default',
                                agentId: agentId ?? '',
                                envId: env.name,
                            }),
                        label: env.displayName ?? env.name,
                        status: deployments?.[env.name]?.status as TabStatus,
                        isProduction: env.isProduction,
                        id: env.name,
                    };
                })}
                selectedId={envId}
            />
        </Box>
    );
};
