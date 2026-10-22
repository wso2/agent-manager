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
import { Link } from 'react-router-dom';
import {
  Box,
  Chip,
  Divider,
  Form,
  Stack,
  Tooltip,
  Typography,
} from '@wso2/oxygen-ui';
import type {
  A2ACatalogAgent,
  A2ACatalogEnvironment,
  AgentCardStatus,
} from '@agent-management-platform/types';
import { a2aAgentViewPath } from '../routes';
import type { EnvironmentLabel } from '../utils/useEnvironmentLabel';

export const TILE_TEST_ID = 'a2a-agent-tile';
export const envStatusTestId = (name: string) => `a2a-env-status-${name}`;

const VISIBLE_SKILLS = 2;

const smallChipSx = {
  height: 20,
  fontSize: '0.7rem',
  '& .MuiChip-label': { px: 0.75 },
} as const;

const statusDotColour: Record<AgentCardStatus, string> = {
  fetched: 'success.main',
  pending: 'warning.main',
  failed: 'error.main',
};

const unfetchedCardCaption: Record<AgentCardStatus, string | undefined> = {
  fetched: undefined,
  pending: 'card pending',
  failed: 'card failed',
};

const SkillsRow: React.FC<{ skills: string[] }> = ({ skills }) => {
  const hiddenCount = skills.length - VISIBLE_SKILLS;
  return (
    <Stack
      data-testid="a2a-agent-tile-skills"
      direction="row"
      spacing={0.75}
      sx={{
        alignItems: 'center',
        minWidth: 0,
        overflow: 'hidden',
        flexWrap: 'nowrap',
      }}
    >
      {skills.slice(0, VISIBLE_SKILLS).map((skill, index) => (
        <Chip
          key={`${index}-${skill}`}
          label={skill}
          size="small"
          sx={{ ...smallChipSx, flexShrink: 0 }}
        />
      ))}
      {hiddenCount > 0 && (
        <Tooltip title={skills.join(', ')} placement="top">
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ flexShrink: 0, whiteSpace: 'nowrap' }}
          >
            {`+${hiddenCount} more`}
          </Typography>
        </Tooltip>
      )}
    </Stack>
  );
};

const EnvironmentStatus: React.FC<{
  environment: A2ACatalogEnvironment;
  label: string;
}> = ({ environment, label }) => (
  <Stack
    direction="row"
    spacing={0.5}
    sx={{ alignItems: 'center', flexShrink: 0 }}
  >
    <Box
      data-testid={envStatusTestId(environment.name)}
      title={environment.status}
      sx={{
        width: 8,
        height: 8,
        borderRadius: '50%',
        bgcolor: statusDotColour[environment.status],
      }}
    />
    <Typography variant="caption" color="text.secondary">
      {label}
    </Typography>
    {environment.proxied && (
      <Chip label="proxied" size="small" sx={smallChipSx} />
    )}
    {environment.cardDiffers && (
      <Chip
        label="card differs"
        size="small"
        color="warning"
        sx={smallChipSx}
      />
    )}
  </Stack>
);

export interface A2AAgentTileProps {
  orgId: string;
  agent: A2ACatalogAgent;
  emphasised: boolean;
  environmentLabel: EnvironmentLabel;
}

export const A2AAgentTile: React.FC<A2AAgentTileProps> = ({
  orgId,
  agent,
  emphasised,
  environmentLabel,
}) => {
  const title = agent.name || agent.agentName;
  const lead = agent.environments.find(
    (environment) => environment.name === agent.leadEnvironment,
  );
  const leadCaption = lead && unfetchedCardCaption[lead.status];

  return (
    <Link
      to={a2aAgentViewPath({
        orgId,
        projectName: agent.projectName,
        agentName: agent.agentName,
      })}
      style={{ textDecoration: 'none' }}
      data-testid={TILE_TEST_ID}
      data-emphasised={String(emphasised)}
    >
      <Form.CardButton
        selected={emphasised}
        sx={{
          width: '100%',
          height: '100%',
          minHeight: 200,
          textAlign: 'left',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'flex-start',
        }}
      >
        <Form.CardHeader
          sx={{
            width: '100%',
            minWidth: 0,
            overflow: 'hidden',
            '& .MuiCardHeader-content': { minWidth: 0 },
          }}
          title={
            <Stack direction="column" spacing={1} sx={{ minWidth: 0 }}>
              <Tooltip title={title} placement="top">
                <Typography variant="h6" noWrap sx={{ minWidth: 0 }}>
                  {title}
                </Typography>
              </Tooltip>
              <Stack
                direction="row"
                spacing={1}
                sx={{ alignItems: 'center', minWidth: 0 }}
              >
                <Chip
                  label={agent.kind}
                  size="small"
                  variant="outlined"
                  color="primary"
                  sx={smallChipSx}
                />
                <Divider
                  orientation="vertical"
                  flexItem
                  sx={{ height: 16, alignSelf: 'center' }}
                />
                <Typography variant="caption" color="text.secondary" noWrap>
                  {agent.projectName}
                </Typography>
                {leadCaption && (
                  <Typography variant="caption" color="warning.main" noWrap>
                    {leadCaption}
                  </Typography>
                )}
              </Stack>
            </Stack>
          }
        />
        <Form.CardContent
          sx={{
            width: '100%',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            flexGrow: 1,
            gap: 1,
            pb: 2,
            '&.MuiCardContent-root:last-child': { pb: 2 },
          }}
        >
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
            }}
          >
            {agent.description}
          </Typography>
          {agent.skills.length > 0 && <SkillsRow skills={agent.skills} />}
          <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 1.5 }}>
            {agent.environments.map((environment) => (
              <EnvironmentStatus
                key={environment.name}
                environment={environment}
                label={environmentLabel(environment.name)}
              />
            ))}
          </Stack>
        </Form.CardContent>
      </Form.CardButton>
    </Link>
  );
};
