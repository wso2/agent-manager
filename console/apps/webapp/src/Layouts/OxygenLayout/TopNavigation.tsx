import {
  useListAgents,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  ComplexSelect,
  Header,
  IconButton,
  Menu,
  MenuItem,
} from "@wso2/oxygen-ui";
import { Bot, Folder, Plus, XCircle } from "@wso2/oxygen-ui-icons-react";
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
              <>
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
                        <XCircle size={18} />
                      </IconButton>
              <ComplexSelect
                value={projectId}
                size="small"
                sx={{ minWidth: 180 }}
                renderValue={() => (
                  <>
                    <ComplexSelect.MenuItem.Icon>
                      <Folder size={20} />
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
                <ComplexSelect.ListHeader>
                  Projects List
                </ComplexSelect.ListHeader>
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
                      <Folder size={20} />
                    </ComplexSelect.MenuItem.Icon>
                    <ComplexSelect.MenuItem.Text
                      primary={project.displayName}
                    />
                  </ComplexSelect.MenuItem>
                ))}
              </ComplexSelect>
         
              </>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setProjectAnchorEl(e.currentTarget)}
                  size="small"
                >
                  <Folder size={20} />
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
                      <Folder size={20} style={{ marginRight: 8 }} />
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
              <>
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
                  <XCircle size={18} />
                </IconButton>
                <ComplexSelect
                  value={agentId}
                  size="small"
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
                  <ComplexSelect.ListHeader>Agents List</ComplexSelect.ListHeader>
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
                      <ComplexSelect.MenuItem.Text primary={agent.displayName} />
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
          
              </>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setAgentAnchorEl(e.currentTarget)}
                  size="small"
                >
                  <Bot size={20} />
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
                      {agent.displayName}
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
