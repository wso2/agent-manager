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

import { Box, Button, MenuItem, Select, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { Bot, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useGetAgentRegistryEntry } from "@agent-management-platform/api-client";
import { LabelChips } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type AgentRegistryEntryDetail,
  type AgentRegistryEnvironmentDetail,
} from "@agent-management-platform/types";
import { Link, generatePath, useLocation, useParams, useSearchParams } from "react-router-dom";
import {
  AgentPageColumns,
  Property,
  PropertiesSidebar,
  SidebarHeading,
  TimeAgo,
} from "../AgentPage/AgentPageParts";
import { agentTypeText } from "../AgentRegistry/subComponents/agentRow";
import { DeploymentStatusChip } from "../AgentRegistry/subComponents/DeploymentStatusChip";
import { UnknownOwnerLabel } from "../AgentRegistry/subComponents/UnknownOwnerLabel";
import { AboutCard } from "./subComponents/AboutCard";
import { GovernanceCard } from "./subComponents/Governance";

const registryRoute = absoluteRouteMap.children.org.children.agentRegistry;

const ENV_PARAM = "env";

/**
 * The environment the page shows: the one in the URL, else the first in the agent's
 * pipeline (the lowest, e.g. Development). Undefined when the agent has no environments.
 */
function chosenEnvironment(
  agent: AgentRegistryEntryDetail,
  fromUrl: string | null,
): AgentRegistryEnvironmentDetail | undefined {
  return agent.environments.find((e) => e.name === fromUrl) ?? agent.environments[0];
}

/**
 * The environment dropdown in the page header. The chosen one reads "Development
 * Environment", as the environment selectors on the project pages do.
 */
function EnvironmentSelect({
  environments,
  value,
  onChange,
}: {
  environments: AgentRegistryEnvironmentDetail[];
  value: string;
  onChange: (name: string) => void;
}) {
  const displayName = (name: string) =>
    environments.find((e) => e.name === name)?.displayName ?? name;
  return (
    <Select
      size="small"
      variant="outlined"
      value={value}
      onChange={(e) => onChange(e.target.value as string)}
      renderValue={(name) => `${displayName(name as string)} Environment`}
      inputProps={{ "aria-label": "Environment" }}
      sx={{ minWidth: 200 }}
    >
      {environments.map((env) => (
        <MenuItem key={env.name} value={env.name}>
          {env.displayName}
        </MenuItem>
      ))}
    </Select>
  );
}

/**
 * The sidebar: the agent's status in the environment chosen at the top, then what it is
 * whatever the environment, and its tags.
 */
function Sidebar({
  agent,
  env,
}: {
  agent: AgentRegistryEntryDetail;
  env: AgentRegistryEnvironmentDetail | undefined;
}) {
  const hasLabels = Object.keys(agent.labels).length > 0;
  return (
    <PropertiesSidebar>
      {env && (
        <Box>
          <SidebarHeading title="Status" />
          <Stack spacing={1.25}>
            <Property label="Deployment">
              <DeploymentStatusChip status={env.deploymentStatus} />
            </Property>
            <Property label="Last deployed">
              {env.lastDeployedAt ? (
                <TimeAgo iso={env.lastDeployedAt} />
              ) : (
                // External agents are never deployed by Agent Manager.
                <Typography variant="body2" color="text.secondary" component="span">
                  {agent.hosting === "external" ? "Not deployed by Agent Manager" : "Never"}
                </Typography>
              )}
            </Property>
          </Stack>
        </Box>
      )}
      <Box>
        <SidebarHeading title="Details" />
        <Stack spacing={1.25}>
          <Property label="Project">{agent.projectDisplayName}</Property>
          <Property label="Type">{agentTypeText(agent)}</Property>
          <Property label="Created">
            <TimeAgo iso={agent.createdAt} />
          </Property>
          <Property label="Created by">
            {agent.createdBy ? agent.createdBy.displayName : <UnknownOwnerLabel />}
          </Property>
        </Stack>
      </Box>
      <Box>
        <SidebarHeading title="Tags" />
        {hasLabels ? (
          <LabelChips labels={agent.labels} />
        ) : (
          <Typography variant="body2" color="text.secondary">
            None
          </Typography>
        )}
      </Box>
    </PropertiesSidebar>
  );
}

/**
 * Org-level overview of one managed agent, laid out like an unmanaged agent's page:
 * a main column with what changes per environment (chosen in the header) and a sidebar
 * with what does not. Links on to its project page.
 */
export function AgentRegistryOverview() {
  const {
    orgId = "",
    registryProjectId = "",
    registryAgentId = "",
  } = useParams<{ orgId: string; registryProjectId: string; registryAgentId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const { data: agent, isLoading, error } = useGetAgentRegistryEntry({
    orgName: orgId,
    projName: registryProjectId,
    agentName: registryAgentId,
  });

  // The list as it was when this page was opened; the plain list on a direct visit.
  const backHref = useReturnTo(generatePath(registryRoute.path, { orgId }));
  const pageProps = { backHref, backLabel: "Back", avatar: null };

  if (isLoading) {
    return (
      <PageLayout title={registryAgentId} isLoading {...pageProps}>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={160} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  if (error || !agent) {
    return (
      <PageLayout title="Agent not found" {...pageProps}>
        <StateNotice
          icon={Bot}
          tone={error ? "error" : "info"}
          title={error ? "Couldn't load the agent" : "Agent not found"}
          description={
            error
              ? "The registry could not load this agent. Try again later."
              : `There is no agent "${registryAgentId}" in project "${registryProjectId}".`
          }
          actions={
            <Button variant="contained" component={Link} to={backHref}>
              Go to agent registry
            </Button>
          }
        />
      </PageLayout>
    );
  }

  const env = chosenEnvironment(agent, searchParams.get(ENV_PARAM));
  // Replace, not push, and keep the router state so Back still returns to the list.
  const chooseEnvironment = (name: string) =>
    setSearchParams({ [ENV_PARAM]: name }, { replace: true, state: location.state });

  const projectAgentPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.path,
    { orgId, projectId: agent.projectName, agentId: agent.name },
  );

  return (
    <PageLayout
      title={agent.displayName}
      // One line: what it is and where. The description, which can be long, is in About.
      description={`${agentTypeText(agent)} agent in ${agent.projectDisplayName}`}
      {...pageProps}
      actions={
        <Stack direction="row" spacing={1} alignItems="center">
          {env && (
            <EnvironmentSelect
              environments={agent.environments}
              value={env.name}
              onChange={chooseEnvironment}
            />
          )}
          <Button
            variant="contained"
            component={Link}
            to={projectAgentPath}
            startIcon={<ExternalLink size={16} />}
          >
            Open in project
          </Button>
        </Stack>
      }
    >
      <AgentPageColumns
        main={
          <>
            <AboutCard agent={agent} env={env} />
            {env && <GovernanceCard hosting={agent.hosting} env={env} />}
          </>
        }
        sidebar={<Sidebar agent={agent} env={env} />}
      />
    </PageLayout>
  );
}

export default AgentRegistryOverview;
