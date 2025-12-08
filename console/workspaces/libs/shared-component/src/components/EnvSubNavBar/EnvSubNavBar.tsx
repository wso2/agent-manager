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
