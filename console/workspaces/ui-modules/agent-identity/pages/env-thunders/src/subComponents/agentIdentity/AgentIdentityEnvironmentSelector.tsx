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

import { useMemo } from "react";
import { FormControl, MenuItem, Select, Typography } from "@wso2/oxygen-ui";
import { useAgentIdentityEnvironmentOptions } from "./useAgentIdentityEnvironmentOptions";

/**
 * Environment selector for the org-level Agents page. Lists every
 * environment in the org (not just ones with a provisioned identity) and
 * updates the `envName` search param on change.
 */
export function AgentIdentityEnvironmentSelector() {
  const { envName, options, handleChange } = useAgentIdentityEnvironmentOptions();

  const selectedEnvironment = useMemo(
    () => options.find((env) => env.name === envName),
    [options, envName],
  );

  if (options.length <= 1) {
    return null;
  }

  return (
    <FormControl size="small" sx={{ minWidth: 160 }}>
      <Select
        value={envName}
        onChange={(e) => handleChange(e.target.value as string)}
        renderValue={(value) => (
          <Typography>
            {selectedEnvironment?.displayName ?? value}
            {" "}
            Environment
          </Typography>
        )}
      >
        {options.map((env) => (
          <MenuItem key={env.name} value={env.name}>
            {env.displayName ?? env.name}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
}
