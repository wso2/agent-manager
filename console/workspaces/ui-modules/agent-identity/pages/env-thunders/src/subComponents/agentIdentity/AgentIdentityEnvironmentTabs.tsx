/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ReactNode } from "react";
import { Box, Tab, Tabs } from "@wso2/oxygen-ui";
import { useAgentIdentityEnvironmentOptions } from "./useAgentIdentityEnvironmentOptions";

/**
 * Environment picker for the Groups/Roles list pages, rendered as tabs at the
 * top of the listing card instead of a header-action dropdown so the current
 * environment reads as a prominent, primary choice rather than a form field.
 * Mirrors AgentIdentityEnvironmentSelector's behavior/data source (shared via
 * useAgentIdentityEnvironmentOptions) — only the rendered control differs.
 */
interface AgentIdentityEnvironmentTabsProps {
  /** Shown at the right end of the tab bar, e.g. the list's create button. */
  action?: ReactNode;
}

export function AgentIdentityEnvironmentTabs({ action }: AgentIdentityEnvironmentTabsProps) {
  const { envName, options, handleChange } = useAgentIdentityEnvironmentOptions();

  const tabs =
    options.length > 1 ? (
      <Tabs
        value={envName}
        onChange={(_e, newEnvName: string) => handleChange(newEnvName)}
        sx={{ flex: 1, minWidth: 0 }}
      >
        {options.map((env) => (
          <Tab key={env.name} value={env.name} label={env.displayName ?? env.name} />
        ))}
      </Tabs>
    ) : (
      <Box sx={{ flex: 1 }} />
    );

  if (options.length <= 1 && !action) {
    return null;
  }

  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 2,
        px: 2,
        py: action ? 1 : 0,
        borderBottom: 1,
        borderColor: "divider",
      }}
    >
      {tabs}
      {action}
    </Box>
  );
}
