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

import { useState } from 'react';
import { FormControl, MenuItem, Select, Stack, Typography } from '@wso2/oxygen-ui';
import { useGetAgentCard } from '@agent-management-platform/api-client';
import {
  a2aCardsIdentical,
  diffA2ACards,
} from '@agent-management-platform/shared-component';
import type { AgentPathParams } from '@agent-management-platform/types';
import { cardState, type CardState } from './cardState';
import type { EnvironmentLabel } from '../utils/useEnvironmentLabel';

export const COMPARE_SELECT_LABEL = 'Compare with';

const COMPARE_LABEL_ID = 'a2a-compare-label';

export interface A2ACardCompareProps {
  params: AgentPathParams;
  /** The selected environment's card body. */
  baseCard: Record<string, unknown>;
  /** In entry order, without the selected environment. */
  otherEnvironments: string[];
  environmentLabel: EnvironmentLabel;
}

function DiffLines({
  baseCard,
  otherCard,
}: {
  baseCard: Record<string, unknown>;
  otherCard: Record<string, unknown>;
}) {
  const diff = diffA2ACards(baseCard, otherCard);
  if (a2aCardsIdentical(diff)) {
    return (
      <Typography variant="caption" color="text.secondary">
        The cards are identical.
      </Typography>
    );
  }
  const lines = [
    ...diff.skills.added.map((name) => `Added: ${name}`),
    ...diff.skills.removed.map((name) => `Removed: ${name}`),
    ...diff.skills.changed.map((name) => `Changed: ${name}`),
    ...(diff.fields.length > 0 ? [`Also differs in: ${diff.fields.join(', ')}`] : []),
  ];
  return (
    <Stack spacing={0.5}>
      {lines.map((line, index) => (
        <Typography key={`${index}-${line}`} variant="body2">
          {line}
        </Typography>
      ))}
    </Stack>
  );
}

function CompareResult({
  baseCard,
  compare,
}: {
  baseCard: Record<string, unknown>;
  compare: CardState;
}) {
  switch (compare.state) {
    case 'loading':
      return <Typography variant="body2">Loading…</Typography>;
    case 'error':
      return <Typography variant="body2">Couldn&apos;t load the card to compare.</Typography>;
    case 'absent':
    case 'noBody':
      return <Typography variant="body2">No fetched card to compare.</Typography>;
    case 'ready':
      return <DiffLines baseCard={baseCard} otherCard={compare.card} />;
  }
}

/** Diffs the selected environment's card against another environment's, chosen here. */
export function A2ACardCompare({
  params,
  baseCard,
  otherEnvironments,
  environmentLabel,
}: A2ACardCompareProps) {
  const [selected, setSelected] = useState('');
  const compareQuery = useGetAgentCard(
    { ...params, envId: selected },
    { enabled: selected !== '', poll: false },
  );

  if (otherEnvironments.length === 0) return null;

  return (
    <Stack spacing={1}>
      <Typography id={COMPARE_LABEL_ID} variant="subtitle2" color="text.secondary">
        {COMPARE_SELECT_LABEL}
      </Typography>
      <FormControl size="small" sx={{ maxWidth: 240 }}>
        <Select
          labelId={COMPARE_LABEL_ID}
          displayEmpty
          value={selected}
          onChange={(event) => setSelected(String(event.target.value))}
        >
          <MenuItem value="">None</MenuItem>
          {otherEnvironments.map((name) => (
            <MenuItem key={name} value={name}>
              {environmentLabel(name)}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      {selected !== '' && (
        <CompareResult baseCard={baseCard} compare={cardState(compareQuery)} />
      )}
    </Stack>
  );
}
