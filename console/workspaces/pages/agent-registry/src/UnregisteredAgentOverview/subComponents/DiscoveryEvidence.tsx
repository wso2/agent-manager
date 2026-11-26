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

import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import type { AgentRegistryFindingSignal } from "@agent-management-platform/types";
import { SectionCard, ValueText } from "../../AgentRegistryOverview/subComponents/SectionCard";

/** What the platform call returned for the signal, as "key: value" pairs. */
function observedText(observed: Record<string, unknown>): string {
  return Object.entries(observed)
    .map(([k, v]) => `${k}: ${String(v)}`)
    .join(", ");
}

/** Why discovery counts this as an agent: each signal, the call behind it and what it saw. */
export function DiscoveryEvidence({ signals }: { signals: AgentRegistryFindingSignal[] }) {
  return (
    <SectionCard title="Discovery evidence" subheader="Why Shadow Agent Discovery counts it as an agent">
      {signals.length === 0 ? (
        <ValueText>No signals recorded.</ValueText>
      ) : (
        <Stack spacing={2}>
          {signals.map((signal) => (
            <Box key={signal.code}>
              <Stack direction="row" spacing={1} alignItems="baseline" flexWrap="wrap" useFlexGap>
                <Typography variant="body2" fontWeight={600}>
                  {signal.name}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {formatRelativeTime(signal.detectedAt)}
                </Typography>
              </Stack>
              <Typography
                variant="caption"
                color="text.secondary"
                component="div"
                sx={{ fontFamily: "monospace", overflowWrap: "anywhere", mt: 0.5 }}
              >
                {signal.condition}
              </Typography>
              {Object.keys(signal.observed).length > 0 && (
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  Observed: {observedText(signal.observed)}
                </Typography>
              )}
            </Box>
          ))}
        </Stack>
      )}
    </SectionCard>
  );
}
