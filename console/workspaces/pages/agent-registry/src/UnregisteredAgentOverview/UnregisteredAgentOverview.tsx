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

import { Button, Chip, Grid, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Bot } from "@wso2/oxygen-ui-icons-react";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useGetUnregisteredAgent } from "@agent-management-platform/api-client";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type AgentRegistryDiscovery } from "@agent-management-platform/types";
import { Link, generatePath, useParams } from "react-router-dom";
import { ownerHintText } from "../AgentRegistry/agentRegistryFilters";
import { shadowSourceText } from "../AgentRegistry/subComponents/agentRow";
import {
  Field,
  SectionCard,
  ValueText,
} from "../AgentRegistryOverview/subComponents/SectionCard";
import { DiscoveryEvidence } from "./subComponents/DiscoveryEvidence";
import { PlatformAttributes } from "./subComponents/PlatformAttributes";

const registryRoute = absoluteRouteMap.children.org.children.agentRegistry;

/** The description the platform reported for the agent, if any. */
function platformDescription(discovery: AgentRegistryDiscovery): string | undefined {
  const description = discovery.attributes.description;
  return typeof description === "string" && description.trim() ? description : undefined;
}

/**
 * One row: Platform | Source | Created at the platform | Last seen (| Created by), with
 * Shadow Agent Discovery's labels. Like that page, fields the platform didn't report
 * are left out, not shown empty. The finding's type and the platform's own state (e.g.
 * Bedrock's "PREPARED") are left out too: they are each platform's own words and read
 * as noise here.
 */
function Summary({ discovery }: { discovery: AgentRegistryDiscovery }) {
  const ownerHint = ownerHintText(discovery.ownerHint);
  const createdByLabel = discovery.ownerHint.source
    ? `Created by (${discovery.ownerHint.source})`
    : "Created by";

  const fields: { label: string; value: string }[] = [
    { label: "Platform", value: discovery.providerDisplayName },
    { label: "Source", value: shadowSourceText(discovery) },
  ];
  if (discovery.providerCreatedAt) {
    fields.push({
      label: "Created at the platform",
      value: formatRelativeTime(discovery.providerCreatedAt),
    });
  }
  fields.push({ label: "Last seen", value: formatRelativeTime(discovery.lastSeenAt) });
  if (ownerHint) fields.push({ label: createdByLabel, value: ownerHint });

  // The fields share one row on wide screens; two per row on small ones, one on phones.
  const column = { xs: 12, sm: 6, lg: 12 / fields.length };
  return (
    <SectionCard title="Summary">
      <Grid container spacing={2.5}>
        {fields.map((field) => (
          <Grid key={field.label} size={column}>
            <Field label={field.label}>
              <ValueText>{field.value}</ValueText>
            </Field>
          </Grid>
        ))}
      </Grid>
    </SectionCard>
  );
}

/**
 * Org-level overview of one unregistered (shadow) agent, laid out like a registered
 * agent's: a summary and what its platform reports. No governance section: Agent
 * Manager governs nothing about it. Built from its Shadow Agent Discovery finding.
 */
export function UnregisteredAgentOverview() {
  const { orgId = "", findingId = "" } = useParams<{ orgId: string; findingId: string }>();
  const { data: agent, isLoading, error } = useGetUnregisteredAgent({
    orgName: orgId,
    findingId,
  });

  // The list as it was when this page was opened; the plain list on a direct visit.
  const backHref = useReturnTo(generatePath(registryRoute.path, { orgId }));
  const pageProps = { backHref, backLabel: "Back to agent registry", avatar: null };

  if (isLoading) {
    return (
      <PageLayout title="Unregistered agent" isLoading {...pageProps}>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={160} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  const discovery = agent?.discovery;
  if (error || !agent || !discovery) {
    return (
      <PageLayout title="Agent not found" {...pageProps}>
        <StateNotice
          icon={Bot}
          tone={error ? "error" : "info"}
          title={error ? "Couldn't load the agent" : "Agent not found"}
          description={
            error
              ? "The registry could not load this agent. Try again later."
              : "Shadow Agent Discovery has no agent with this ID."
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

  return (
    <PageLayout
      title={agent.displayName}
      titleTail={<Chip size="small" variant="outlined" color="error" label="Unregistered" />}
      // The agent's own description, which only some platforms report (Anthropic and
      // Foundry put it in attributes); none otherwise. Where it was found is in Summary.
      description={platformDescription(discovery)}
      {...pageProps}
    >
      <Stack spacing={2}>
        <Summary discovery={discovery} />
        <PlatformAttributes discovery={discovery} />
        <DiscoveryEvidence signals={discovery.signals} />
      </Stack>
    </PageLayout>
  );
}

export default UnregisteredAgentOverview;
