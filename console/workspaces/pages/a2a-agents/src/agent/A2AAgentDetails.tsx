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
import { Box, Button, Stack } from '@wso2/oxygen-ui';
import { Plug } from '@wso2/oxygen-ui-icons-react';
import { EMPTY_A2A_CARD, parseA2ACard } from '@agent-management-platform/shared-component';
import { DrawerContent, DrawerHeader, DrawerWrapper } from '@agent-management-platform/views';
import type { A2ACatalogAgent } from '@agent-management-platform/types';
import type { EnvironmentLabel } from '../utils/useEnvironmentLabel';
import { A2ACardCompare } from './A2ACardCompare';
import { A2ACardDetails } from './A2ACardDetails';
import { A2AConnectSection } from './A2AConnectSection';
import type { CardState } from './cardState';

export interface A2AAgentDetailsProps {
  orgId: string;
  entry: A2ACatalogAgent;
  environment: string;
  environmentLabel: EnvironmentLabel;
  /** The selected environment's card state. */
  state: CardState;
  /** Rendered left of the Connect button on one row. */
  chips: ReactNode;
}

/**
 * Everything that resets when the environment changes: the compare selection,
 * the Connect drawer and the test key issued in it. Render it keyed by environment.
 */
export function A2AAgentDetails({
  orgId,
  entry,
  environment,
  environmentLabel,
  state,
  chips,
}: A2AAgentDetailsProps) {
  const [drawerMounted, setDrawerMounted] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const openDrawer = () => {
    setDrawerMounted(true);
    setDrawerOpen(true);
  };
  const closeDrawer = () => setDrawerOpen(false);

  const compare =
    state.state === 'ready' ? (
      <A2ACardCompare
        params={{ orgName: orgId, projName: entry.projectName, agentName: entry.agentName }}
        baseCard={state.card}
        otherEnvironments={entry.environments
          .map(({ name }) => name)
          .filter((name) => name !== environment)}
        environmentLabel={environmentLabel}
      />
    ) : undefined;

  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>{chips}</Box>
        <Button
          variant="contained"
          size="small"
          startIcon={<Plug size={16} />}
          onClick={openDrawer}
          sx={{ flexShrink: 0 }}
        >
          Connect
        </Button>
      </Stack>
      <A2ACardDetails state={state} compare={compare} />
      {drawerMounted && (
        <DrawerWrapper open={drawerOpen} keepMounted maxWidth={700} onClose={closeDrawer}>
          <DrawerHeader
            icon={<Plug size={20} />}
            title={`Connect · ${environmentLabel(environment)}`}
            onClose={closeDrawer}
          />
          <DrawerContent>
            <A2AConnectSection
              orgId={orgId}
              entry={entry}
              environment={environment}
              card={state.state === 'ready' ? parseA2ACard(state.card) : EMPTY_A2A_CARD}
            />
          </DrawerContent>
        </DrawerWrapper>
      )}
    </Stack>
  );
}
