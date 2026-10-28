import {
  useListAgents,
  useListOrganizations,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  Header,
  MenuItem,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { asLink, LevelSwitcherCard } from "./LevelSwitcherCard";
import { useActiveAgentPage, useActiveOrgPage, useActiveProjectPage } from "./path-map";
import { useAuthHooks } from "@agent-management-platform/auth";

const orgLabel = (
  org?: { name: string; displayName: string },
  fallbackName?: string,
) => {
  if (org?.displayName) {
    return org.displayName;
  }
  return fallbackName || org?.name;
};

export function TopNavigation() {
  const navigate = useNavigate();
  const { userInfo } = useAuthHooks();
  const theme = useTheme();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const commonOrgPages = useActiveOrgPage();
  const commonProjectPages = useActiveProjectPage();
  const commonAgentPages = useActiveAgentPage();
  const [orgAnchorEl, setOrgAnchorEl] = useState<null | HTMLElement>(null);

  const [projectAnchorEl, setProjectAnchorEl] = useState<null | HTMLElement>(
    null,
  );

  const [agentAnchorEl, setAgentAnchorEl] = useState<null | HTMLElement>(null);

  // Get all organizations
  const { data: organizations } = useListOrganizations();
  const selectedOrganization = useMemo(() => {
    return organizations?.organizations?.find(
      (organization) => organization.name === orgId,
    );
  }, [organizations, orgId]);
  const hasOtherOrgs = (organizations?.total ?? 0) > 1;
  // With a single org, its displayName may not be set yet — fall back to the
  // org name/handle from the user's own IDP claims rather than the raw org id.
  const soloOrgFallbackName = !hasOtherOrgs
    ? userInfo?.ouName || userInfo?.ouHandle
    : undefined;

  // Get all projects for the organization
  const { data: projects } = useListProjects({
    orgName: orgId,
  });

  const selectedProject = useMemo(() => {
    return projects?.projects?.find((project) => project.name === projectId);
  }, [projects, projectId]);

  // Get all agents for the project
  const { data: agents } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });

  const selectedAgent = useMemo(() => {
    return agents?.agents?.find((agent) => agent.name === agentId);
  }, [agents, agentId]);

  return (
    <>
      <Header.Switchers showDivider={false}>
        {organizations?.organizations && selectedOrganization && (
          <LevelSwitcherCard
            label={hasOtherOrgs ? "Organizations" : "Organization"}
            chevronLabel="Switch organization"
            canSwitch={hasOtherOrgs}
            anchorEl={orgAnchorEl}
            onOpenMenu={(e) => setOrgAnchorEl(e.currentTarget)}
            onCloseMenu={() => setOrgAnchorEl(null)}
            selected={{
              to:
                generatePath(absoluteRouteMap.children.org.path, {
                  orgId: selectedOrganization.name,
                }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
              goToLabel: `Go to ${orgLabel(selectedOrganization, soloOrgFallbackName)}`,
              content: (
                <Typography variant="body1" noWrap sx={{ maxWidth: "100%" }}>
                  {orgLabel(selectedOrganization, soloOrgFallbackName)}
                </Typography>
              ),
            }}
          >
            {hasOtherOrgs &&
              organizations.organizations.map((organization) => (
                <MenuItem
                  key={organization.name}
                  selected={organization.name === orgId}
                  onClick={() => setOrgAnchorEl(null)}
                  {...asLink(
                    generatePath(absoluteRouteMap.children.org.path, {
                      orgId: organization.name,
                    }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
                  )}
                >
                  {orgLabel(organization)}
                </MenuItem>
              ))}
          </LevelSwitcherCard>
        )}

        {projects?.projects && (
          <LevelSwitcherCard
            label="Projects"
            chevronLabel={selectedProject ? "Switch project" : "Select or create a project"}
            anchorEl={projectAnchorEl}
            onOpenMenu={(e) => setProjectAnchorEl(e.currentTarget)}
            onCloseMenu={() => setProjectAnchorEl(null)}
            selected={
              selectedProject && {
                to:
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.path,
                    { orgId, projectId },
                  ) + (commonProjectPages ? `/${commonProjectPages}` : ""),
                goToLabel: `Go to ${selectedProject.displayName}`,
                closeLabel: "Close project",
                onClose: () =>
                  navigate(
                    generatePath(absoluteRouteMap.children.org.path, { orgId }),
                  ),
                content: (
                  <Typography variant="body1" noWrap sx={{ maxWidth: "100%" }}>
                    {selectedProject.displayName}
                  </Typography>
                ),
              }
            }
          >
            <MenuItem
              onClick={() => setProjectAnchorEl(null)}
              {...asLink(
                generatePath(
                  absoluteRouteMap.children.org.children.newProject.path,
                  { orgId },
                ),
              )}
            >
              <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
              Create a Project
            </MenuItem>
            {projects.projects.map((project) => (
              <MenuItem
                key={project.name}
                selected={project.name === projectId}
                onClick={() => setProjectAnchorEl(null)}
                {...asLink(
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.path,
                    { orgId, projectId: project.name },
                  ) + (commonProjectPages ? `/${commonProjectPages}` : ""),
                )}
              >
                {project.displayName}
              </MenuItem>
            ))}
          </LevelSwitcherCard>
        )}

        {agents?.agents && (
          <LevelSwitcherCard
            label="Agents"
            chevronLabel={selectedAgent ? "Switch agent" : "Select or create an agent"}
            anchorEl={agentAnchorEl}
            onOpenMenu={(e) => setAgentAnchorEl(e.currentTarget)}
            onCloseMenu={() => setAgentAnchorEl(null)}
            selected={
              selectedAgent && {
                to:
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.path,
                    { orgId, projectId, agentId },
                  ) + (commonAgentPages ? `/${commonAgentPages}` : ""),
                goToLabel: `Go to ${selectedAgent.displayName}`,
                closeLabel: "Close agent",
                onClose: () =>
                  navigate(
                    generatePath(
                      absoluteRouteMap.children.org.children.projects.path,
                      { orgId, projectId },
                    ),
                  ),
                content: (
                  <Stack
                    direction="row"
                    gap={1}
                    alignItems="center"
                    sx={{ maxWidth: "100%", minWidth: 0 }}
                  >
                    <Typography variant="body1" noWrap sx={{ minWidth: 0 }}>
                      {selectedAgent.displayName}
                    </Typography>
                  </Stack>
                ),
              }
            }
          >
            <MenuItem
              onClick={() => setAgentAnchorEl(null)}
              {...asLink(
                generatePath(
                  absoluteRouteMap.children.org.children.projects.children
                    .newAgent.path,
                  { orgId, projectId },
                ),
              )}
            >
              <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
              Create an Agent
            </MenuItem>
            {agents.agents.map((agent) => (
              <MenuItem
                key={agent.name}
                selected={agent.name === agentId}
                onClick={() => setAgentAnchorEl(null)}
                {...asLink(
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.path,
                    { orgId, projectId, agentId: agent.name },
                  ) + (commonAgentPages ? `/${commonAgentPages}` : ""),
                )}
              >
                <Stack direction="row" gap={1} alignItems="center">
                  {agent.displayName}
                </Stack>
              </MenuItem>
            ))}
          </LevelSwitcherCard>
        )}
      </Header.Switchers>
    </>
  );
}
