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

import { IsolationTierBadge } from "@agent-management-platform/shared-component";
import type { Environment } from "@agent-management-platform/types";
import { Box, Tab, Tabs } from "@wso2/oxygen-ui";

interface EnvironmentTabsBarProps {
  environments: Environment[];
  selectedName?: string;
  onSelect: (name: string) => void;
  /** Theme color path (e.g. "success.main") for the small dot on each tab. */
  dotColor: (environment: Environment) => string;
}

/**
 * Tab strip that switches between an agent's pipeline environments. Rendered
 * as an EnvironmentCard's `tabsHeader`, so it sits inside the card, in the
 * same row as that environment's status/sandbox tier.
 */
export function EnvironmentTabsBar({
  environments,
  selectedName,
  onSelect,
  dotColor,
}: EnvironmentTabsBarProps) {
  if (environments.length === 0) {
    return null;
  }

  const value = environments.some((env) => env.name === selectedName) ? selectedName : false;

  return (
    <Tabs
      value={value}
      onChange={(_, name: string) => onSelect(name)}
      variant="scrollable"
      scrollButtons="auto"
      sx={{ minHeight: 0 }}
    >
      {environments.map((env) => (
        <Tab
          key={env.name}
          value={env.name}
          sx={{ minHeight: 0, py: 0.5 }}
          label={
            <Box display="flex" alignItems="center" gap={0.75}>
              <IsolationTierBadge
                tier={env.isolationTier}
                size={14}
                color={env.name === selectedName ? "primary.main" : undefined}
              />
              {env.displayName ?? env.name}
              <Box
                sx={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  bgcolor: dotColor(env),
                  flexShrink: 0,
                }}
              />
            </Box>
          }
        />
      ))}
    </Tabs>
  );
}
