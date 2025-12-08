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

import { Box } from "@wso2/oxygen-ui";
import { MessageCircle as ChatBubbleIcon, BarChart3 as AnalyticsOutlined } from '@wso2/oxygen-ui-icons-react';
import { GroupNavLinks, SubTopNavBar } from "./SubTopNavBar";
import { generatePath, matchPath, Outlet, useLocation, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const EnvSubNavBar = () => {
    const { orgId, projectId, agentId, envId } = useParams();
    const { pathname } = useLocation();
    const navLinks: GroupNavLinks[] = [
        {
            id: 'overview',
            navLinks: [
                {
                    id: 'Try Out',
                    label: 'Try Out',
                    icon: <ChatBubbleIcon />,
                    isActive: !!matchPath(
                        absoluteRouteMap.children.org.children.projects.children.
                            agents.children.environment.path, pathname),
                    path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.path, { orgId: orgId ?? 'default', projectId: projectId ?? 'default', agentId: agentId ?? 'default', envId: envId ?? 'default' })
                },
                {
                    id: 'observe',
                    label: 'Observe',
                    icon: <AnalyticsOutlined />,
                    isActive: !!matchPath(
                        absoluteRouteMap.children.org.children.projects.children.
                            agents.children.environment.children.observability.wildPath, pathname),
                    path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.observability.children.traces.path, { orgId: orgId ?? 'default', projectId: projectId ?? 'default', agentId: agentId ?? 'default', envId: envId ?? 'default' })
                }
            ]
        }
    ];
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <SubTopNavBar navLinks={navLinks} />
            <Outlet />
        </Box>
    );
};
