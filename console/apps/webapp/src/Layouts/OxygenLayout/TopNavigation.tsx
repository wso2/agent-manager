import {
  useListAgents,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  Box,
  Chip,
  ComplexSelect,
  Header,
  IconButton,
  Menu,
  MenuItem,
  Stack,
} from "@wso2/oxygen-ui";
import { Bot, ChevronRightCircle, Package, Plus, X } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";

export function TopNavigation() {
  const navigate = useNavigate();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const [projectAnchorEl, setProjectAnchorEl] = useState<null | HTMLElement>(
    null,
  );
  const projectMenuOpen = Boolean(projectAnchorEl);

  const [agentAnchorEl, setAgentAnchorEl] = useState<null | HTMLElement>(null);
  const agentMenuOpen = Boolean(agentAnchorEl);

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
        {projects?.projects && (
          <>

            {selectedProject ? (
              <Box position="relative">

                <ComplexSelect
                  value={projectId}
                  size="small"
                  sx={{ minWidth: 180 }}
                  label="Projects"
                  renderValue={() => (
                    <>
                      <ComplexSelect.MenuItem.Icon>
                        <Package size={20} />
                      </ComplexSelect.MenuItem.Icon>
                      <ComplexSelect.MenuItem.Text
                        primary={selectedProject?.displayName}
                      />
                    </>
                  )}
                  onChange={(e) => {
                    const selectedProjectName = e.target.value as string;
                    navigate(
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.path,
                        { orgId, projectId: selectedProjectName },
                      ),
                    );
                  }}
                >
                  <ComplexSelect.MenuItem
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.newProject.path,
                          { orgId },
                        ),
                      );
                    }}
                  >
                    <ComplexSelect.MenuItem.Icon>
                      <Plus size={20} />
                    </ComplexSelect.MenuItem.Icon>
                    <ComplexSelect.MenuItem.Text primary="Create a Project" />
                  </ComplexSelect.MenuItem>
                  {projects.projects.map((project) => (
                    <ComplexSelect.MenuItem
                      key={project.name}
                      value={project.name}

                    >
                      <ComplexSelect.MenuItem.Icon>
                        <Package size={20} />
                      </ComplexSelect.MenuItem.Icon>
                      <ComplexSelect.MenuItem.Text
                        primary={project.displayName}
                      />
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                <Box position="absolute" right={0} top={-2}>
                <IconButton
                  size="small"
                  onClick={() => {
                    navigate(
                      generatePath(absoluteRouteMap.children.org.path, {
                        orgId,
                      }),
                    );
                  }}
                >
                  <X size={12} />
                </IconButton>
                </Box>
              </Box>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setProjectAnchorEl(e.currentTarget)}
                  size="small"
                  sx={{
                    transform: projectMenuOpen ? "rotate(90deg)" : "rotate(0deg)",
                    transition: "transform 0.2s",
                  }}
                >
                  <ChevronRightCircle size={20} />
                </IconButton>
                <Menu
                  anchorEl={projectAnchorEl}
                  open={projectMenuOpen}
                  onClose={() => setProjectAnchorEl(null)}
                >
                  <MenuItem
                    onClick={() => {
                      setProjectAnchorEl(null);
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.newProject
                            .path,
                          { orgId },
                        ),
                      );
                    }}
                  >
                    <Plus size={20} style={{ marginRight: 8 }} />
                    Create a Project
                  </MenuItem>
                  {projects.projects.map((project) => (
                    <MenuItem
                      key={project.name}
                      onClick={() => {
                        setProjectAnchorEl(null);
                        navigate(
                          generatePath(
                            absoluteRouteMap.children.org.children.projects
                              .path,
                            { orgId, projectId: project.name },
                          ),
                        );
                      }}
                    >
                      <Package size={20} style={{ marginRight: 8 }} />
                      {project.displayName}
                    </MenuItem>
                  ))}
                </Menu>

              </>
            )}

          </>
        )}

        {agents?.agents && (
          <>
            {selectedAgent ? (
              <Box position="relative">
                <ComplexSelect
                  value={agentId}
                  size="small"
                  label="Agents"
                  sx={{ minWidth: 180 }}
                  renderValue={() => (
                    <>
                      <ComplexSelect.MenuItem.Icon>
                        <Bot size={20} />
                      </ComplexSelect.MenuItem.Icon>
                      <ComplexSelect.MenuItem.Text
                        primary={selectedAgent?.displayName}
                      />
                    </>
                  )}
                  onChange={(e) => {
                    const selectedAgentName = e.target.value as string;
                    navigate(
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .agents.path,
                        { orgId, projectId, agentId: selectedAgentName },
                      ),
                    );
                  }}
                >
                  <ComplexSelect.MenuItem
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects.children
                            .newAgent.path,
                          { orgId, projectId },
                        ),
                      );
                    }}
                  >
                    <ComplexSelect.MenuItem.Icon>
                      <Plus size={20} />
                    </ComplexSelect.MenuItem.Icon>
                    <ComplexSelect.MenuItem.Text primary="Create an Agent" />
                  </ComplexSelect.MenuItem>
                  {agents.agents.map((agent) => (
                    <ComplexSelect.MenuItem key={agent.name} value={agent.name}>
                      <ComplexSelect.MenuItem.Icon>
                        <Bot size={20} />
                      </ComplexSelect.MenuItem.Icon>
                      <ComplexSelect.MenuItem.Text primary={
                        <Stack direction="row" gap={1} alignItems="center">
                          {agent.displayName}
                          {
                            agent.provisioning.type === 'external' && (
                              <Chip
                                label={'External'}
                                size="small"
                                variant="outlined"
                              />
                            )
                          }
                        </Stack>
                      } />
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                <Box position="absolute" right={0} top={-2}>
                  <IconButton
                    size="small"
                    onClick={() => {
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects.path,
                          { orgId, projectId },
                        ),
                      );
                    }}
                  >
                    <X size={12} />
                  </IconButton>
                </Box>
              </Box>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setAgentAnchorEl(e.currentTarget)}
                  size="small"
                  sx={{
                    transform: agentMenuOpen ? "rotate(90deg)" : "rotate(0deg)",
                    transition: "transform 0.2s",
                  }}
                >
                  <ChevronRightCircle size={20} />
                </IconButton>
                <Menu
                  anchorEl={agentAnchorEl}
                  open={agentMenuOpen}
                  onClose={() => setAgentAnchorEl(null)}
                >
                  <MenuItem
                    onClick={() => {
                      setAgentAnchorEl(null);
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects
                            .children.newAgent.path,
                          { orgId, projectId },
                        ),
                      );
                    }}
                  >
                    <Plus size={20} style={{ marginRight: 8 }} />
                    Create an Agent
                  </MenuItem>
                  {agents.agents.map((agent) => (
                    <MenuItem
                      key={agent.name}
                      onClick={() => {
                        setAgentAnchorEl(null);
                        navigate(
                          generatePath(
                            absoluteRouteMap.children.org.children.projects
                              .children.agents.path,
                            { orgId, projectId, agentId: agent.name },
                          ),
                        );
                      }}
                    >
                      <Bot size={20} style={{ marginRight: 8 }} />
                      <Stack direction="row" gap={1} alignItems="center">
                        {agent.displayName}
                        {
                          agent.provisioning.type === 'external' && (
                            <Chip
                              label={'External'}
                              size="small"
                              variant="outlined"
                            />
                          )
                        }
                      </Stack>
                    </MenuItem>
                  ))}
                </Menu>
              </>
            )}
          </>
        )}
      </Header.Switchers>
    </>
  );
}
