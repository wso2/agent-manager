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

import type { ReactNode } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Divider,
  Grid,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Bot, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useGetAgentRegistryEntry } from "@agent-management-platform/api-client";
import { LabelChips, formatRelativeTime } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type AgentRegistryEntry } from "@agent-management-platform/types";
import { Link, generatePath, useParams } from "react-router-dom";
import { agentTypeText } from "../AgentRegistry/subComponents/agentRow";
import { DeploymentStatusChip } from "../AgentRegistry/subComponents/DeploymentStatusChip";
import { UnknownOwnerLabel } from "../AgentRegistry/subComponents/UnknownOwnerLabel";

const registryRoute = absoluteRouteMap.children.org.children.agentRegistry;

/** Outlined, titled card for one section of the page. */
function SectionCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card variant="outlined">
      <CardHeader title={title} />
      <Divider />
      <CardContent>{children}</CardContent>
    </Card>
  );
}

/** One label and its value in the summary. */
function SummaryItem({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Grid size={{ xs: 12, sm: 6, lg: 4 }}>
      <Typography variant="caption" color="text.secondary" component="div">
        {label}
      </Typography>
      <Box sx={{ mt: 0.5 }}>{children}</Box>
    </Grid>
  );
}

function Summary({ agent }: { agent: AgentRegistryEntry }) {
  const hasLabels = Object.keys(agent.labels).length > 0;
  return (
    <SectionCard title="Summary">
      <Grid container spacing={2.5}>
        <SummaryItem label="Project">
          <Typography variant="body2">{agent.projectDisplayName}</Typography>
        </SummaryItem>
        <SummaryItem label="Type">
          <Typography variant="body2">{agentTypeText(agent)}</Typography>
        </SummaryItem>
        <SummaryItem label="Owner">
          <Typography variant="body2">
            {agent.createdBy ? agent.createdBy.displayName : <UnknownOwnerLabel />}
          </Typography>
        </SummaryItem>
        <SummaryItem label="Created">
          <Typography variant="body2">{formatRelativeTime(agent.createdAt)}</Typography>
        </SummaryItem>
        <SummaryItem label="Tags">
          {hasLabels ? (
            <LabelChips labels={agent.labels} />
          ) : (
            <Typography variant="body2" color="text.secondary">
              None
            </Typography>
          )}
        </SummaryItem>
      </Grid>
    </SectionCard>
  );
}

/** Names of the resources, or "None". */
function ResourceNames({ names }: { names: string[] }) {
  return (
    <Typography variant="body2" color={names.length ? "text.primary" : "text.secondary"}>
      {names.length ? names.join(", ") : "None"}
    </Typography>
  );
}

/** Per environment: deployment status, when it was last deployed, and what it can reach. */
function Environments({ agent }: { agent: AgentRegistryEntry }) {
  return (
    <SectionCard title="Environments">
      {/* Scrolls inside the card on narrow screens. */}
      <Box sx={{ overflowX: "auto" }}>
        <Box sx={{ minWidth: 640 }}>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Environment</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
                <ListingTable.Cell>Last deployed</ListingTable.Cell>
                <ListingTable.Cell>LLM providers</ListingTable.Cell>
                <ListingTable.Cell>MCP servers</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {agent.environments.map((env) => (
                <ListingTable.Row key={env.name}>
                  <ListingTable.Cell>
                    <Typography variant="body2">{env.displayName}</Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <DeploymentStatusChip status={env.deploymentStatus} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2" color="text.secondary">
                      {env.lastDeployedAt ? formatRelativeTime(env.lastDeployedAt) : "—"}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <ResourceNames names={env.llmProviders.map((p) => p.displayName)} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <ResourceNames names={env.mcpServers.map((s) => s.displayName)} />
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </Box>
      </Box>
    </SectionCard>
  );
}

/**
 * Org-level overview of one registered agent. What it shows is still to be decided;
 * for now a summary and its environments, with a link to the agent's project page.
 */
export function AgentRegistryOverview() {
  const {
    orgId = "",
    registryProjectId = "",
    registryAgentId = "",
  } = useParams<{ orgId: string; registryProjectId: string; registryAgentId: string }>();
  const { data: agent, isLoading, error } = useGetAgentRegistryEntry({
    orgName: orgId,
    projName: registryProjectId,
    agentName: registryAgentId,
  });

  // The list as it was when this page was opened; the plain list on a direct visit.
  const backHref = useReturnTo(generatePath(registryRoute.path, { orgId }));
  const pageProps = { backHref, backLabel: "Back to agent registry", avatar: null };

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
        <Button
          variant="contained"
          component={Link}
          to={projectAgentPath}
          startIcon={<ExternalLink size={16} />}
        >
          Open in project
        </Button>
      }
    >
      <Stack spacing={2}>
        <Summary agent={agent} />
        <Environments agent={agent} />
      </Stack>
    </PageLayout>
  );
}

export default AgentRegistryOverview;
