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

import { useState, type ReactNode } from 'react';
import { Box, Button, Stack, Typography } from '@wso2/oxygen-ui';
import {
  A2ASkillItem,
  CodeBlock,
  formatRelativeTime,
  parseA2ACard,
  type A2ACardCapabilities,
} from '@agent-management-platform/shared-component';
import type { AgentCardResponse } from '@agent-management-platform/types';
import type { CardState } from './cardState';

export const CARD_JSON_FIELD_ID = 'a2a-agent-card-json';

const CAPABILITY_KEYS: Array<keyof A2ACardCapabilities> = [
  'streaming',
  'pushNotifications',
  'extendedAgentCard',
];

export interface A2ACardDetailsProps {
  state: CardState;
  /** Rendered between Capabilities and the raw-JSON toggle, ready state only. */
  compare?: ReactNode;
}

function Message({ children }: { children: ReactNode }) {
  return (
    <Typography variant="body2" color="text.secondary">
      {children}
    </Typography>
  );
}

function StatusBlock({ response }: { response: AgentCardResponse }) {
  if (response.status === 'fetched') {
    return (
      <Typography variant="caption" color="text.secondary">
        Fetched {formatRelativeTime(response.fetchedAt)}
      </Typography>
    );
  }
  return (
    <Box>
      <Typography variant="body2" color="warning.main">
        {response.status === 'pending' ? 'Card pending' : 'Card fetch failed'}
      </Typography>
      {response.lastError && (
        <Typography variant="caption" color="error.main">
          {response.lastError}
        </Typography>
      )}
    </Box>
  );
}

function RawJson({ card }: { card: Record<string, unknown> }) {
  const [shown, setShown] = useState(false);
  return (
    <Stack spacing={1} alignItems="flex-start">
      <Button variant="text" size="small" onClick={() => setShown((current) => !current)}>
        {shown ? 'Hide raw JSON' : 'View raw JSON'}
      </Button>
      {shown && (
        <Box sx={{ alignSelf: 'stretch' }}>
          <CodeBlock
            code={JSON.stringify(card, null, 2)}
            language="json"
            fieldId={CARD_JSON_FIELD_ID}
          />
        </Box>
      )}
    </Stack>
  );
}

function ReadyCard({ card, compare }: { card: Record<string, unknown>; compare?: ReactNode }) {
  const parsed = parseA2ACard(card);
  return (
    <>
      {parsed.description && <Typography variant="body2">{parsed.description}</Typography>}
      <Stack spacing={1}>
        <Typography variant="subtitle2" color="text.secondary">
          Skills
        </Typography>
        {parsed.skills.length === 0 ? (
          <Message>No skills declared.</Message>
        ) : (
          <Stack spacing={1}>
            {parsed.skills.map((skill, index) => (
              <A2ASkillItem key={`${index}-${skill.id || skill.name}`} skill={skill} />
            ))}
          </Stack>
        )}
      </Stack>
      <Stack spacing={0.5}>
        <Typography variant="subtitle2" color="text.secondary">
          Capabilities
        </Typography>
        {CAPABILITY_KEYS.map((key) => (
          <Typography key={key} variant="body2">
            {parsed.capabilities[key] ? '✓' : '✗'} {key}
          </Typography>
        ))}
      </Stack>
      {compare}
      <RawJson card={card} />
    </>
  );
}

/** The selected environment's card; skills, capabilities, compare and raw JSON need a body. */
export function A2ACardDetails({ state, compare }: A2ACardDetailsProps) {
  switch (state.state) {
    case 'loading':
      return <Message>Loading card…</Message>;
    case 'absent':
      return <Message>No agent card is configured for this environment.</Message>;
    case 'error':
      return <Message>Couldn&apos;t load the agent card.</Message>;
    case 'noBody':
      return <StatusBlock response={state.response} />;
    case 'ready':
      return (
        <Stack spacing={2}>
          <StatusBlock response={state.response} />
          <ReadyCard card={state.card} compare={compare} />
        </Stack>
      );
  }
}
