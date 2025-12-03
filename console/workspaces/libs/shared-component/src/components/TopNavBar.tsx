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

import { Box } from "@mui/material";
import { useParams, useSearchParams } from "react-router-dom";
import { TabStatus, TopNavBarGroup } from "./LinkTab";
import { useGetAgent, useListAgentDeployments, useListEnvironments } from "@agent-management-platform/api-client";
import { useEffect, useMemo } from "react";


export const TopNavBar: React.FC = () => {
    const { orgId, agentId, projectId } = useParams();
    const [searchParams, setSearchParams] = useSearchParams();
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

    const selectedEnvironment = searchParams.get('environment');

    // Set first environment as default if no environment is selected
    useEffect(() => {
        if (!selectedEnvironment && sortedEnvironments.length > 0 && agent?.provisioning.type === 'internal') {
            setSearchParams(prev => {
                const newSearchParams = new URLSearchParams(prev);
                newSearchParams.set('environment', sortedEnvironments[0].name);
                return newSearchParams; 
            }, { replace: true });
        }
    }, [selectedEnvironment, sortedEnvironments, setSearchParams, agent?.provisioning.type]);

    if (agent?.provisioning.type === 'external' || !agent) {
        return null;
    }

    return (
        <Box display="flex" gap={1}>
            <TopNavBarGroup
                // eslint-disable-next-line max-len
                tabs={sortedEnvironments.map((env) => {
                    const tabSearchParams = new URLSearchParams(searchParams);
                    tabSearchParams.set('environment', env.name);
                    return {
                        to: `?${tabSearchParams.toString()}`,
                        label: env.displayName ?? env.name,
                        status: deployments?.[env.name]?.status as TabStatus,
                        isProduction: env.isProduction,
                        id: env.name,
                    };
                })}
                selectedId={selectedEnvironment || sortedEnvironments[0]?.name}
            />
        </Box>
    );
};
