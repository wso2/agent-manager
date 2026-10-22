/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from 'react';
import { Alert, Box, Chip, Stack, Typography } from '@wso2/oxygen-ui';
import { useParams } from 'react-router-dom';
import { useGetA2ACatalogEntry, useGetAgentCard } from '@agent-management-platform/api-client';
import { normalizeVersion, parseA2ACard } from '@agent-management-platform/shared-component';
import { PageLayout } from '@agent-management-platform/views';
import type { A2ACatalogAgent } from '@agent-management-platform/types';
import { A2AAgentDetails } from './agent/A2AAgentDetails';
import { cardState, type CardState } from './agent/cardState';
import { a2aAgentsListPath, type A2AAgentRouteParams } from './routes';
import { useEnvironmentLabel, type EnvironmentLabel } from './utils/useEnvironmentLabel';
import { useReplaceSearchParams } from './utils/useReplaceSearchParams';

export const A2A_AGENT_PAGE_TEST_ID = 'a2a-agent-page';

/** The `env` query value when the entry lists it, else the entry's lead environment. */
function selectEnvironment(entry: A2ACatalogAgent | null | undefined, requested: string): string {
  if (!entry) return '';
  return entry.environments.some(({ name }) => name === requested)
    ? requested
    : entry.leadEnvironment;
}

interface HeaderIdentity {
  title: string;
  version: string;
  providerOrganization: string;
}

/** Title and version come together from the ready card, else from the entry. */
function headerIdentity(entry: A2ACatalogAgent, state: CardState): HeaderIdentity {
  if (state.state !== 'ready') {
    return { title: entry.name, version: entry.version, providerOrganization: '' };
  }
  const card = parseA2ACard(state.card);
  return {
    title: card.name || entry.name,
    version: card.version,
    providerOrganization: card.providerOrganization,
  };
}

interface EnvironmentChipsProps {
  entry: A2ACatalogAgent;
  selected: string;
  environmentLabel: EnvironmentLabel;
  onSelect: (environment: string) => void;
}

function EnvironmentChips({ entry, selected, environmentLabel, onSelect }: EnvironmentChipsProps) {
  return entry.environments.map(({ name }) => {
    const isSelected = name === selected;
    return (
      <Chip
        key={name}
        label={environmentLabel(name)}
        color={isSelected ? 'primary' : 'default'}
        variant={isSelected ? 'filled' : 'outlined'}
        onClick={() => onSelect(name)}
      />
    );
  });
}

export const A2AAgentPage: React.FC = () => {
  const {
    orgId = '',
    projectName = '',
    agentName = '',
  } = useParams<keyof A2AAgentRouteParams>();
  const entryQuery = useGetA2ACatalogEntry({ orgName: orgId, projName: projectName, agentName });
  const entry = entryQuery.data;
  const [params, replaceParams] = useReplaceSearchParams();
  const environment = selectEnvironment(entry, params.get('env') ?? '');
  const environmentLabel = useEnvironmentLabel(orgId);
  const cardQuery = useGetAgentCard(
    { orgName: orgId, projName: projectName, agentName, envId: environment },
    { enabled: !!entry && environment !== '' },
  );
  const state = cardState(cardQuery);
  const identity = entry ? headerIdentity(entry, state) : null;

  const titleTail = entry && identity ? (
    <Stack direction="row" spacing={1} alignItems="center">
      {identity.version !== '' && (
        <Chip size="small" variant="outlined" label={normalizeVersion(identity.version)} />
      )}
      <Chip size="small" label={cardQuery.data?.source ?? entry.kind} />
    </Stack>
  ) : null;

  const description = identity?.providerOrganization
    ? `${projectName} · ${identity.providerOrganization}`
    : projectName;

  const selectEnvironmentChip = (name: string) => replaceParams((p) => p.set('env', name));

  const renderBody = () => {
    if (entry === null) {
      return (
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          This agent was not found in the catalogue.
        </Typography>
      );
    }
    if (!entry) {
      return entryQuery.isError ? (
        <Alert severity="error">Failed to load the agent. Please try again.</Alert>
      ) : null;
    }
    return (
      <Stack spacing={2}>
        {entryQuery.isError ? (
          <Alert severity="warning">Couldn&apos;t refresh; showing the last loaded details.</Alert>
        ) : null}
        {environment !== '' && (
          <A2AAgentDetails
            key={environment}
            orgId={orgId}
            entry={entry}
            environment={environment}
            environmentLabel={environmentLabel}
            state={state}
            chips={
              <EnvironmentChips
                entry={entry}
                selected={environment}
                environmentLabel={environmentLabel}
                onSelect={selectEnvironmentChip}
              />
            }
          />
        )}
      </Stack>
    );
  };

  return (
    <PageLayout
      title={identity?.title ?? agentName}
      titleTail={titleTail}
      description={description}
      avatar={null}
      backHref={a2aAgentsListPath(orgId)}
      backLabel="Back to A2A Agents"
      isLoading={entryQuery.isLoading && !entry}
    >
      <Box data-testid={A2A_AGENT_PAGE_TEST_ID}>{renderBody()}</Box>
    </PageLayout>
  );
};
