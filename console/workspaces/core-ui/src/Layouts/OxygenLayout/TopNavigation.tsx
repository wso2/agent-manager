import {
  orgRouteId,
  useListAgents,
  useListOrganizations,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  Header,
  HeaderBreadcrumbs,
  type HeaderBreadcrumbsItem,
  type HeaderBreadcrumbsLevel,
} from "@wso2/oxygen-ui";
import { AppWindow, Box, Building2 } from "@wso2/oxygen-ui-icons-react";
import { forwardRef, useMemo, type ComponentPropsWithoutRef } from "react";
import { generatePath, Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { useActiveAgentPage, useActiveOrgPage, useActiveProjectPage } from "./path-map";
import { useAuthHooks } from "@agent-management-platform/auth";
import { returnToState } from "@agent-management-platform/views";

const orgLabel = (
  org?: { name: string; displayName: string },
  fallbackName?: string,
) => {
  if (org?.displayName) {
    return org.displayName;
  }
  return fallbackName || org?.name;
};

/**
 * HeaderBreadcrumbs hands link targets over as `href`; react-router's Link
 * wants `to`. Adapting here keeps menu entries real anchors (middle-click,
 * open-in-new-tab) while navigating client-side.
 */
const RouterLink = forwardRef<
  HTMLAnchorElement,
  Omit<ComponentPropsWithoutRef<typeof Link>, "to"> & { href: string }
>(function RouterLink({ href, ...rest }, ref) {
  return <Link ref={ref} to={href} {...rest} />;
});

const withSuffix = (path: string, suffix?: string | null) =>
  suffix ? `${path}/${suffix}` : path;

export function TopNavigation() {
  const { userInfo } = useAuthHooks();
  const navigate = useNavigate();
  const location = useLocation();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const commonOrgPages = useActiveOrgPage();
  const commonProjectPages = useActiveProjectPage();
  const commonAgentPages = useActiveAgentPage();

  const { data: organizations } = useListOrganizations();
  const selectedOrganization = useMemo(
    () =>
      organizations?.organizations?.find(
        (organization) => orgRouteId(organization) === orgId,
      ),
    [organizations, orgId],
  );
  const hasOtherOrgs = (organizations?.total ?? 0) > 1;
  // With a single org, its displayName may not be set yet — fall back to the
  // org name/handle from the user's own IDP claims rather than the raw org id.
  const soloOrgFallbackName = !hasOtherOrgs
    ? userInfo?.ouName || userInfo?.ouHandle
    : undefined;

  const { data: projects } = useListProjects({ orgName: orgId });
  const selectedProject = useMemo(
    () => projects?.list?.find((project) => project.metadata.name === projectId),
    [projects, projectId],
  );

  const { data: agents } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const selectedAgent = useMemo(
    () => agents?.agents?.find((agent) => agent.name === agentId),
    [agents, agentId],
  );

  const levels = useMemo(() => {
    const result: HeaderBreadcrumbsLevel[] = [];

    if (organizations?.organizations && selectedOrganization) {
      const toItem = (
        org: (typeof organizations.organizations)[number],
        fallback?: string,
      ): HeaderBreadcrumbsItem => ({
        id: orgRouteId(org),
        name: orgLabel(org, fallback) ?? orgRouteId(org),
        href: withSuffix(
          generatePath(absoluteRouteMap.children.org.path, {
            orgId: orgRouteId(org),
          }),
          commonOrgPages,
        ),
      });
      result.push({
        id: "organization",
        label: "Organization",
        icon: <Building2 size={20} />,
        pluralLabel: "Organizations",
        // A single org has nothing to switch to, so the menu lists only it.
        items: hasOtherOrgs
          ? organizations.organizations.map((org) => toItem(org))
          : [toItem(selectedOrganization, soloOrgFallbackName)],
        selectedId: orgRouteId(selectedOrganization),
        createAction: false,
        manageAction: false,
      });
    }

    // Create pages return to where they were opened from, so the link is
    // followed in code with the current location as router state.
    const openCreatePage =
      (href: string) => (event: { preventDefault: () => void }) => {
        event.preventDefault();
        navigate(href, { state: returnToState(location) });
      };

    if (projects?.list && orgId) {
      const newProjectPath = generatePath(
        absoluteRouteMap.children.org.children.newProject.path,
        { orgId },
      );
      result.push({
        id: "project",
        label: "Project",
        icon: <Box size={20} />,
        items: projects.list.map((project) => ({
          id: project.metadata.name,
          name: project.spec.displayName,
          href: withSuffix(
            generatePath(absoluteRouteMap.children.org.children.projects.path, {
              orgId,
              projectId: project.metadata.name,
            }),
            commonProjectPages,
          ),
        })),
        selectedId: selectedProject?.metadata.name,
        openLabel: "Select or create a project",
        createAction: {
          label: "Create a Project",
          href: newProjectPath,
          onClick: openCreatePage(newProjectPath),
        },
        manageAction: false,
      });
    }

    if (agents?.agents && orgId && projectId) {
      const newAgentPath = generatePath(
        absoluteRouteMap.children.org.children.projects.children.newAgent.path,
        { orgId, projectId },
      );
      result.push({
        id: "agent",
        label: "Agent",
        icon: <AppWindow size={20} />,
        items: agents.agents.map((agent) => ({
          id: agent.name,
          name: agent.displayName,
          href: withSuffix(
            generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .path,
              { orgId, projectId, agentId: agent.name },
            ),
            commonAgentPages,
          ),
        })),
        selectedId: selectedAgent?.name,
        openLabel: "Select or create an agent",
        createAction: {
          label: "Create an Agent",
          href: newAgentPath,
          onClick: openCreatePage(newAgentPath),
        },
        manageAction: false,
      });
    }

    return result;
  }, [
    organizations,
    selectedOrganization,
    hasOtherOrgs,
    soloOrgFallbackName,
    projects,
    selectedProject,
    agents,
    selectedAgent,
    orgId,
    projectId,
    commonOrgPages,
    commonProjectPages,
    commonAgentPages,
    navigate,
    location,
  ]);

  return (
    <Header.Switchers showDivider={false}>
      <HeaderBreadcrumbs
        levels={levels}
        linkComponent={RouterLink}
        onNavigate={(_levelId, item) => {
          if (item.href) {
            navigate(item.href);
          }
        }}
        showIcons
        aria-label="Organization, project and agent"
      />
    </Header.Switchers>
  );
}
