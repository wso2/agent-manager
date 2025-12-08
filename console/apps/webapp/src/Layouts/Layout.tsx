import { useAuthHooks } from '@agent-management-platform/auth';
import { Outlet, useParams, useNavigate, generatePath } from "react-router-dom";
import { Box } from '@wso2/oxygen-ui';
import { useNavigationItems } from './navigationItems';
import { createUserMenuItems } from './userMenuItems';
import { displayProvisionTypes, MainLayout } from '@agent-management-platform/views';
import { useListAgents, useListOrganizations, useListProjects } from '@agent-management-platform/api-client';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { useMemo } from 'react';

export function Layout() {
  const { userInfo, logout } = useAuthHooks();
  const { orgId, projectId, agentId } = useParams<
    { orgId: string, projectId: string, agentId: string }>();
  const navigate = useNavigate();
  const navigationItems = useNavigationItems();
  
  const { data: organizations } = useListOrganizations();

  const homePath = useMemo(() => {
    return generatePath(absoluteRouteMap.children.org.path, { orgId: organizations?.organizations?.[0]?.name ?? '' });
  }, [organizations]);  

  // Get all projects for the organization
  const { data: projects } = useListProjects({
    orgName: orgId ?? '',
  });

  // Get all agents for the project
  const { data: agents } = useListAgents({
    orgName: orgId ?? '',
    projName: projectId ?? '',
  });

  return (
    <MainLayout
      sidebarCollapsed={false}
      user={{
        name: userInfo?.displayName ?? userInfo?.username ?? '',
        email: userInfo?.username ?? userInfo?.orgHandle ?? '',
        avatar: userInfo?.username ?? '',
      }}
      homePath={homePath}
      topSelectorsProps={[
        {
          label: 'Project',
          selectedId: projectId,
          options: projects?.projects?.map((project) => ({
            id: project.name,
            label: project.displayName,
          })) ?? [],
          onChange: (value) => {
            navigate(generatePath(
              absoluteRouteMap.children.org.children.projects.path, { orgId, projectId: value }));
          },
          disableClose: false,
          onClick: () => {
            navigate(generatePath(
              absoluteRouteMap.children.org.children.projects.path, { orgId, projectId }));
          },
          onClose: () => {
            navigate(generatePath(absoluteRouteMap.children.org.path, { orgId }));
          },
        },
        {
        label: 'Agent',
        selectedId: agentId,
        options: agents?.agents?.map((agent) => ({
          id: agent.name,
          label: agent.displayName,
          typeLabel: displayProvisionTypes(agent.provisioning.type),
        })) ?? [],
        onChange: (value) => {
          navigate(generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path,
            { orgId, projectId, agentId: value }))
        },
        disableClose: false,
        onClick: () => {
          navigate(generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path,
            { orgId, projectId }))
        },
        onCreate: () => {
          navigate(generatePath(
            absoluteRouteMap.children.org.children.projects.children.newAgent.path, 
            { orgId, projectId }));
        },
        onClose: () => {
          navigate(generatePath(
            absoluteRouteMap.children.org.children.projects.path, { orgId, projectId }));
        },
      }
    ]}
      userMenuItems={createUserMenuItems(orgId ?? '', logout)}
      navigationItems={navigationItems}
    >
      <Box
        p={1}
        flexGrow={1}
      >
        <Outlet />
      </Box>
    </MainLayout>
  );
}
