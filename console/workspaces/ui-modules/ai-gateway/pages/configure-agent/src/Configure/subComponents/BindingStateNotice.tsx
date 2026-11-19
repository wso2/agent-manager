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

import { Alert } from "@wso2/oxygen-ui";
import { useGetAgentConfigurations } from "@agent-management-platform/api-client";
import type { AgentBinding } from "@agent-management-platform/types";

// Bindings chosen at agent creation are provisioned in the background, so a
// fresh agent can briefly show no configurations. This says so, and surfaces
// a binding the gateway keeps failing to provision.
const DEFAULT_ENVIRONMENT = "default";

const describe = (b: AgentBinding) =>
  `${b.kind === "llm" ? "LLM provider" : "MCP proxy"} ${b.target}`;

export function BindingStateNotice({
  orgName,
  projName,
  agentName,
}: {
  orgName?: string;
  projName?: string;
  agentName?: string;
}) {
  const { data } = useGetAgentConfigurations(
    { orgName, projName, agentName },
    { environment: DEFAULT_ENVIRONMENT },
  );
  const bindings = data?.bindings ?? [];
  const failed = bindings.filter((b) => b.state === "failed");
  const pending = bindings.filter((b) => b.state === "pending");

  if (failed.length > 0) {
    return (
      <Alert severity="warning" sx={{ mb: 2 }}>
        {failed
          .map((b) => `Setting up ${describe(b)} failed and is retried${b.lastError ? `: ${b.lastError}` : "."}`)
          .join(" ")}
      </Alert>
    );
  }
  if (pending.length > 0) {
    return (
      <Alert severity="info" sx={{ mb: 2 }}>
        {`Setting up ${pending.map(describe).join(", ")}. It appears here once ready.`}
      </Alert>
    );
  }
  return null;
}
