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

import { Box, Chip, CircularProgress, IconButton, Stack, Typography } from "@wso2/oxygen-ui";
import { ExternalLink, Plus } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";
import { useGetAgent, useGetAgentCard } from "@agent-management-platform/api-client";
import { OverviewSectionCard, validateEndpointUrl } from "@agent-management-platform/shared-component";
import { buildConfigureTabHref, TextInput } from "@agent-management-platform/views";

interface EnvAgentCardSummaryProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
  external?: boolean;
}

type Json = Record<string, unknown>;

const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const asString = (v: unknown): string | undefined => (typeof v === "string" && v !== "" ? v : undefined);
const objectsIn = (v: unknown): Json[] => (Array.isArray(v) ? v.filter(isObject) : []);

/** Card content is third-party: read it defensively and link only http(s) URLs. */
function summarizeCard(card: Json) {
  const provider = isObject(card.provider) ? card.provider : {};
  const providerUrl = asString(provider.url);
  return {
    organization: asString(provider.organization),
    providerUrl: providerUrl && validateEndpointUrl(providerUrl) === null ? providerUrl : undefined,
    interfaces: objectsIn(card.supportedInterfaces).flatMap((iface) => {
      const url = asString(iface.url);
      return url ? [{ url, binding: asString(iface.protocolBinding) }] : [];
    }),
    skillNames: objectsIn(card.skills).flatMap((skill) => asString(skill.name) ?? []),
  };
}

function CardSummary({ card }: { card: Json }) {
  const { organization, providerUrl, interfaces, skillNames } = summarizeCard(card);
  if (!organization && interfaces.length === 0 && skillNames.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        The agent card lists no provider, interfaces or skills.
      </Typography>
    );
  }
  return (
    <Stack spacing={1.5}>
      {organization && (
        <Box>
          <Typography variant="overline" color="text.secondary" display="block">Provider</Typography>
          <Box display="flex" alignItems="center" gap={0.5}>
            <Typography variant="body2">{organization}</Typography>
            {providerUrl && (
              <IconButton
                size="small"
                component="a"
                href={providerUrl}
                target="_blank"
                rel="noopener noreferrer"
                sx={{ p: 0.25 }}
                aria-label="Open provider website"
              >
                <ExternalLink size={13} />
              </IconButton>
            )}
          </Box>
        </Box>
      )}
      {interfaces.length > 0 && (
        <Box>
          <Typography variant="overline" color="text.secondary" display="block" sx={{ mb: 0.5 }}>Interfaces</Typography>
          <Stack spacing={1}>
            {interfaces.map(({ url, binding }, i) => (
              <Box key={`${i}-${url}`} display="flex" gap={1.5} alignItems="center">
                {binding && <Chip size="small" label={binding} sx={{ minWidth: 96 }} />}
                <TextInput
                  value={url}
                  copyable
                  copyTooltipText="Copy URL"
                  size="small"
                  slotProps={{ input: { readOnly: true, sx: { fontFamily: "monospace" } } }}
                />
              </Box>
            ))}
          </Stack>
        </Box>
      )}
      {skillNames.length > 0 && (
        <Box>
          <Typography variant="overline" color="text.secondary" display="block" sx={{ mb: 0.5 }}>Skills</Typography>
          <Box display="flex" gap={0.75} flexWrap="wrap">
            {skillNames.map((name, i) => <Chip key={`${i}-${name}`} size="small" variant="outlined" label={name} />)}
          </Box>
        </Box>
      )}
    </Stack>
  );
}

/** Provider, interfaces and skill names from an A2A card; details live on Configure → A2A. */
export function EnvAgentCardSummary({
  orgId, projectId, agentId, envId, external,
}: EnvAgentCardSummaryProps) {
  const { data: agent } = useGetAgent({ orgName: orgId, projName: projectId, agentName: agentId });
  const isA2A = agent?.agentType?.subType === "a2a-agent";
  const { data, isLoading, isError } = useGetAgentCard(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: isA2A },
  );

  if (!isA2A) {
    return null;
  }
  const configureHref = buildConfigureTabHref(orgId, projectId, agentId, "a2a", envId);
  const noSource = data === null;
  const card = isObject(data?.card) ? data.card : undefined;

  let body;
  if (isLoading) {
    body = <CircularProgress size={16} />;
  } else if (noSource && external) {
    body = (
      <Chip
        component={Link}
        to={configureHref}
        clickable
        variant="outlined"
        color="primary"
        icon={<Plus size={14} />}
        label="Register agent card URL"
      />
    );
  } else if (isError && !noSource) {
    body = <Typography variant="body2" color="error">Unable to load the agent card. Try again later.</Typography>;
  } else if (!card) {
    body = <Typography variant="body2" color="text.secondary">Agent card not available yet.</Typography>;
  } else {
    body = <CardSummary card={card} />;
  }

  return (
    <OverviewSectionCard title="Agent Card" actionHref={configureHref} actionLabel="Configure" sx={{ mb: 1.5 }}>
      {body}
    </OverviewSectionCard>
  );
}
