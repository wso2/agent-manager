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

import { useSearchParams } from "react-router-dom";
import { Alert, CircularProgress, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import { CONFIGURE_ENV_PARAM } from "../../configureTabs";
import { A2AAgentCardPanel } from "./A2AAgentCardPanel";

interface A2AConfigureTabProps {
  orgId: string;
  projectId: string;
  agentId: string;
  external: boolean;
}

/** Per-environment A2A agent card settings. */
export function A2AConfigureTab({ orgId, projectId, agentId, external }: A2AConfigureTabProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const { environments, isLoading, isError } = usePipelineEnvironmentsState(orgId, projectId);

  if (isLoading) {
    return <CircularProgress size={20} />;
  }
  if (isError) {
    return <Alert severity="error">Failed to load environments. Please try again.</Alert>;
  }
  if (environments.length === 0) {
    return <Typography variant="body2" color="text.secondary">No environments are set up for this project.</Typography>;
  }

  const requestedEnv = searchParams.get(CONFIGURE_ENV_PARAM);
  const envIndex = Math.max(0, environments.findIndex((env) => env.name === requestedEnv));
  const envName = environments[envIndex].name;
  const selectEnv = (index: number) =>
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set(CONFIGURE_ENV_PARAM, environments[index].name);
      return next;
    }, { replace: true });

  return (
    <>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {external
          ? "If this agent speaks A2A, register its agent card URL in each environment it serves A2A from."
          : "The platform fetches this agent's card from its gateway in each environment."}
      </Typography>
      <Tabs
        value={envIndex}
        onChange={(_, index: number) => selectEnv(index)}
        sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}
      >
        {environments.map((env) => (
          <Tab key={env.name} label={env.displayName ?? env.name} />
        ))}
      </Tabs>
      <A2AAgentCardPanel
        key={envName}
        orgId={orgId}
        projectId={projectId}
        agentId={agentId}
        envId={envName}
        external={external}
      />
    </>
  );
}
