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

import { generatePath, Outlet, useParams } from "react-router-dom";
import { EnvBuildSelector } from "./EnvBuildSelector";
import { PageLayout } from "@agent-management-platform/views";
import { useGetAgent } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";

export function AgentNavBar() {
    const { orgId, agentId, projectId } = useParams();
    const { data: agent } = useGetAgent({
        orgName: orgId ?? 'default',
        projName: projectId ?? 'default',
        agentName: agentId ?? '',
    });
    return (
        <PageLayout
            title={agent?.displayName ?? 'Agent'}
            description={agent?.description}
            backHref={
                generatePath(absoluteRouteMap.children.org.children.projects.path, { orgId: orgId ?? 'default', projectId: projectId ?? 'default'})
            }
            backLabel="Agents"
            actions={
                <EnvBuildSelector />
            }>
            <Outlet />
        </PageLayout>
    );
}
