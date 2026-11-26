/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useState } from "react";
import {
  Alert,
  AlertTitle,
  Button,
  Card,
  CardContent,
  Grid,
  MenuItem,
  Stack,
  Typography,
  readHeaderBreadcrumbsState,
} from "@wso2/oxygen-ui";
import { Bot, Box } from "@wso2/oxygen-ui-icons-react";
import { Link, generatePath, useLocation, useNavigate } from "react-router-dom";
import {
  LoadErrorNotice,
  PageLayout,
  StateNotice,
  TextInput,
  returnToState,
} from "@agent-management-platform/views";
import {
  useListAllAgents,
  useListAllProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  AGENT_AVAILABLE_FOR,
  agentKindLabel,
  gatePath,
  type LevelGateFrom,
  type NavEntry,
  type NavLevel,
  type NavParams,
} from "../../../Layouts/OxygenLayout/navigation/navModel";

interface LevelPickerProps {
  entry: NavEntry;
  from: NavLevel;
  to: NavLevel;
  params: NavParams;
}

/** The project the top bar last remembered for this org, if any. */
function rememberedProject(orgId?: string): string | undefined {
  try {
    const stored = readHeaderBreadcrumbsState()?.value;
    return stored?.organization === orgId ? (stored?.project ?? undefined) : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Shown when a menu item that lives deeper is opened higher up: asks for the
 * project and/or agent, then opens the item there.
 */
export function LevelPicker({ entry, from, to, params }: LevelPickerProps) {
  const { orgId } = params;
  const navigate = useNavigate();
  const location = useLocation();
  // Set by the top bar when the user moved up from a project or agent.
  const origin = (location.state as { from?: LevelGateFrom } | null)?.from;
  const needProject = from === "org";
  const needAgent = to === "agent";

  const [pickedProject, setPickedProject] = useState("");
  const [pickedAgent, setPickedAgent] = useState("");

  // Every page, so a project or agent past the first page can still be picked.
  const projectsQuery = useListAllProjects({ orgName: needProject ? orgId : undefined });
  const projects = projectsQuery.data?.projects ?? [];
  // Preselect the project the user came from, the remembered one, or the only one.
  const fallbackProject =
    projects.find((p) => p.name === origin?.projectId)?.name ??
    projects.find((p) => p.name === rememberedProject(orgId))?.name ??
    (projects.length === 1 ? projects[0].name : "");
  // An explicit choice wins over the default, but only while it is still listed.
  const validPick = projects.some((p) => p.name === pickedProject) ? pickedProject : "";
  const projectId = needProject ? validPick || fallbackProject : params.projectId;

  const agentsQuery = useListAllAgents({
    orgName: orgId,
    projName: needAgent ? projectId || undefined : undefined,
  });
  const agents = agentsQuery.data?.agents ?? [];
  const supportedAgents = agents.filter((agent) => !entry.unsupportedReason?.(agent));
  // Preselect the agent the user came from (if this page applies to it), or the only one.
  const cameFrom =
    projectId === origin?.projectId
      ? supportedAgents.find((agent) => agent.name === origin?.agentId)?.name
      : undefined;
  const agentId =
    (supportedAgents.some((agent) => agent.name === pickedAgent) ? pickedAgent : "") ||
    cameFrom ||
    (supportedAgents.length === 1 ? supportedAgents[0].name : "");

  // Agents are only trusted from a successful load, not a stale cache after an error.
  const ready = !!projectId && (!needAgent || (!!agentId && !agentsQuery.isError));
  const open = () => {
    // The target level's gate opens the page itself (and picks its environment).
    navigate(gatePath(to, { orgId, projectId, agentId }, entry.id));
  };

  const what = [needProject && "a project", needAgent && "an agent"]
    .filter(Boolean)
    .join(" and ");
  const header = {
    title: entry.label,
    avatar: null,
    description: `${entry.label} is opened for ${to === "agent" ? "an agent" : "a project"}. Choose ${what} to continue.`,
  };

  if (needProject && projectsQuery.isError) {
    return (
      <PageLayout {...header}>
        <LoadErrorNotice
          entityLabel="projects"
          error={projectsQuery.error}
          onRetry={() => projectsQuery.refetch()}
        />
      </PageLayout>
    );
  }
  if (needProject && !projectsQuery.isLoading && projects.length === 0) {
    return (
      <PageLayout {...header}>
        <StateNotice
          icon={Box}
          title="No projects yet"
          description={`Create a project${needAgent ? " and add an agent to it" : ""} to open ${entry.label}.`}
          actions={
            <Button
              variant="contained"
              component={Link}
              to={generatePath(absoluteRouteMap.children.org.children.newProject.path, {
                orgId,
              })}
              state={returnToState(location)}
            >
              Create project
            </Button>
          }
        />
      </PageLayout>
    );
  }

  const agentsLoaded = !!projectId && !agentsQuery.isLoading && !agentsQuery.isError;
  // An empty project gets a create action instead of an empty select. The
  // project select stays (from org level) so the user can try another project.
  const noAgents = needAgent && agentsLoaded && agents.length === 0;
  const noneSupported =
    needAgent && agentsLoaded && agents.length > 0 && supportedAgents.length === 0;
  const availableFor = AGENT_AVAILABLE_FOR[entry.id];
  // Nothing to pick: explain why and offer a way forward, instead of an empty
  // select and a disabled button.
  const blocked = noAgents || noneSupported;
  const orAnotherProject = needProject ? ", or pick another project" : "";
  const blockedTitle = noAgents
    ? "No agents in this project"
    : `No agents in this project support ${entry.label}`;
  const blockedText = noAgents
    ? `Create an agent to open ${entry.label}${orAnotherProject}.`
    : `${entry.label} is only available for ${availableFor ?? "some kinds of agents"}. ` +
      `Create one to use it here${orAnotherProject}.`;
  const createAgentPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.newAgent.path,
    { orgId, projectId },
  );
  // Inside the form (org level) the message stays in the form's flow, under the
  // project select; on its own (project level) it is the page's empty state.
  const agentNotice = !blocked ? null : needProject ? (
    <Alert
      severity="info"
      // Centre the icon, text and button on one axis; the action slot is
      // otherwise top-aligned with its own top padding.
      sx={{ alignItems: "center" }}
      slotProps={{ action: { sx: { alignItems: "center", pt: 0, mr: 0 } } }}
      action={
        <Button
          size="small"
          variant="outlined"
          component={Link}
          to={createAgentPath}
          state={returnToState(location)}
        >
          Create agent
        </Button>
      }
    >
      <AlertTitle>{blockedTitle}</AlertTitle>
      {blockedText}
    </Alert>
  ) : (
    <StateNotice
      icon={Bot}
      title={blockedTitle}
      description={blockedText}
      actions={
        <Button
          variant="contained"
          component={Link}
          to={createAgentPath}
          state={returnToState(location)}
        >
          Create agent
        </Button>
      }
    />
  );

  if (blocked && !needProject) {
    return <PageLayout {...header}>{agentNotice}</PageLayout>;
  }

  return (
    <PageLayout {...header} isLoading={needProject && projectsQuery.isLoading}>
      <Card variant="outlined">
        <CardContent>
          <Stack spacing={3}>
            <Grid container spacing={2}>
              {needProject && (
                <Grid size={{ xs: 12, md: 6, xl: 4 }}>
                  <TextInput
                    select
                    label="Project"
                    value={projectId ?? ""}
                    disabled={projectsQuery.isLoading}
                    onChange={(event) => {
                      setPickedProject(event.target.value);
                      setPickedAgent("");
                    }}
                  >
                    {projects.map((project) => (
                      <MenuItem key={project.name} value={project.name}>
                        {project.displayName || project.name}
                      </MenuItem>
                    ))}
                  </TextInput>
                </Grid>
              )}
              {needAgent && !blocked && (
                <Grid size={{ xs: 12, md: 6, xl: 4 }}>
                  <TextInput
                    select
                    label="Agent"
                    value={agentId}
                    disabled={!projectId || agentsQuery.isLoading}
                    onChange={(event) => setPickedAgent(event.target.value)}
                  >
                    {agents.map((agent) => {
                      // Agents the page doesn't apply to stay listed, greyed
                      // out with their kind, so the project doesn't look empty.
                      const unsupported = !!entry.unsupportedReason?.(agent);
                      return (
                        <MenuItem key={agent.name} value={agent.name} disabled={unsupported}>
                          <Stack
                            direction="row"
                            spacing={1}
                            justifyContent="space-between"
                            sx={{ width: "100%" }}
                          >
                            <span>{agent.displayName || agent.name}</span>
                            {unsupported && (
                              <Typography variant="caption" color="text.secondary">
                                {agentKindLabel(agent)}
                              </Typography>
                            )}
                          </Stack>
                        </MenuItem>
                      );
                    })}
                  </TextInput>
                </Grid>
              )}
            </Grid>
            {needAgent && projectId && agentsQuery.isError && (
              <LoadErrorNotice
                entityLabel="agents"
                error={agentsQuery.error}
                onRetry={() => agentsQuery.refetch()}
              />
            )}
            {agentNotice}
            {needAgent && !blocked && agentsLoaded && agents.length > supportedAgents.length && (
              <Typography variant="body2" color="text.secondary">
                {availableFor
                  ? `${entry.label} is only available for ${availableFor}.`
                  : `Some agents don't support ${entry.label}.`}
              </Typography>
            )}
            {!blocked && (
              <Stack direction="row" justifyContent="flex-start">
                <Button variant="contained" disabled={!ready} onClick={open}>
                  Open {entry.label}
                </Button>
              </Stack>
            )}
          </Stack>
        </CardContent>
      </Card>
    </PageLayout>
  );
}
