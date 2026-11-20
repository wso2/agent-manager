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

import { Button, Grid, MenuItem, Select, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Bot, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useGetAgentRegistryEntry } from "@agent-management-platform/api-client";
import { LabelChips, formatRelativeTime } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type AgentRegistryEntryDetail,
  type AgentRegistryEnvironmentDetail,
} from "@agent-management-platform/types";
import { Link, generatePath, useLocation, useParams, useSearchParams } from "react-router-dom";
import { agentTypeText } from "../AgentRegistry/subComponents/agentRow";
import { DeploymentStatusChip } from "../AgentRegistry/subComponents/DeploymentStatusChip";
import { UnknownOwnerLabel } from "../AgentRegistry/subComponents/UnknownOwnerLabel";
import { Capabilities } from "./subComponents/Capabilities";
import { Governance } from "./subComponents/Governance";
import { Field, SectionCard, ValueText } from "./subComponents/SectionCard";

const registryRoute = absoluteRouteMap.children.org.children.agentRegistry;

const ENV_PARAM = "env";

/**
 * The environment the page shows: the one in the URL, else the first in the agent's
 * pipeline (the lowest, e.g. Development). Undefined when the agent has no environments.
 *
 * The agent's environments are its project's deployment pipeline, in order. In the real
 * API they come from the organization's environments ordered by that pipeline, as
 * usePipelineEnvironmentsState (shared-component) does for project pages; the sample
 * data hardcodes them.
 */
function chosenEnvironment(
  agent: AgentRegistryEntryDetail,
  fromUrl: string | null,
): AgentRegistryEnvironmentDetail | undefined {
  return agent.environments.find((e) => e.name === fromUrl) ?? agent.environments[0];
}

/** The environment dropdown in the page header. */
function EnvironmentSelect({
  environments,
  value,
  onChange,
}: {
  environments: AgentRegistryEnvironmentDetail[];
  value: string;
  onChange: (name: string) => void;
}) {
  return (
    <Select
      size="small"
      variant="outlined"
      value={value}
      onChange={(e) => onChange(e.target.value as string)}
      inputProps={{ "aria-label": "Environment" }}
      sx={{ minWidth: 180 }}
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
 * Four columns: what the agent is | when and by whom it was created | its deployment
 * in the environment chosen at the top | its tags. Columns stack on narrow screens.
 */
function Summary({
  agent,
  env,
}: {
  agent: AgentRegistryEntryDetail;
  env: AgentRegistryEnvironmentDetail | undefined;
}) {
  const hasLabels = Object.keys(agent.labels).length > 0;
  const column = { xs: 12, sm: 6, lg: 3 };
  return (
    <SectionCard title="Summary">
      <Grid container spacing={2.5}>
        <Grid size={column}>
          <Stack spacing={2}>
            <Field label="Project">
              <ValueText>{agent.projectDisplayName}</ValueText>
            </Field>
            <Field label="Type">
              <ValueText>{agentTypeText(agent)}</ValueText>
            </Field>
          </Stack>
        </Grid>
        <Grid size={column}>
          <Stack spacing={2}>
            <Field label="Created on">
              <ValueText>{formatRelativeTime(agent.createdAt)}</ValueText>
            </Field>
            <Field label="Created by">
              <ValueText>
                {agent.createdBy ? agent.createdBy.displayName : <UnknownOwnerLabel />}
              </ValueText>
            </Field>
          </Stack>
        </Grid>
        <Grid size={column}>
          <Stack spacing={2}>
            <Field label="Status">
              {env ? <DeploymentStatusChip status={env.deploymentStatus} /> : <ValueText />}
            </Field>
            <Field label="Last deployed">
              {/* External agents are never deployed by Agent Manager. */}
              <ValueText>
                {env?.lastDeployedAt
                  ? formatRelativeTime(env.lastDeployedAt)
                  : agent.hosting === "external"
                    ? "Not deployed by Agent Manager"
                    : "Never"}
              </ValueText>
            </Field>
          </Stack>
        </Grid>
        <Grid size={column}>
          <Field label="Tags">
            {hasLabels ? <LabelChips labels={agent.labels} /> : <ValueText />}
          </Field>
        </Grid>
      </Grid>
    </SectionCard>
  );
}

/**
 * Org-level overview of one registered agent: a summary, what it declares it can do and
 * its governance, in the environment chosen in the header. Links on to its project page.
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
      description={agent.description}
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
      <Stack spacing={2}>
        <Summary agent={agent} env={env} />
        {env && (
          <>
            <Capabilities agent={agent} env={env} />
            <Governance env={env} />
          </>
        )}
      </Stack>
    </PageLayout>
  );
}

export default AgentRegistryOverview;
