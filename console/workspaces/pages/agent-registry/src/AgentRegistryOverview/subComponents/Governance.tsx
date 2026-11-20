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

import type { ReactNode } from "react";
import { Box, Chip, Grid, Stack, Typography } from "@wso2/oxygen-ui";
import type {
  AgentRegistryEnvironment,
  AgentThunderStatus,
  MonitorStatus,
} from "@agent-management-platform/types";
import { SectionCard } from "./SectionCard";
import { ChipRow, Gap, Tile } from "./Tiles";

const AGENT_ID_LABELS: Record<AgentThunderStatus, string> = {
  completed: "Issued",
  in_progress: "Being issued",
  pending: "Pending",
  failed: "Failed",
};

const AGENT_ID_COLORS: Record<AgentThunderStatus, "success" | "warning" | "error" | "default"> = {
  completed: "success",
  in_progress: "warning",
  pending: "default",
  failed: "error",
};

const MONITOR_COLORS: Record<MonitorStatus, "success" | "default" | "error"> = {
  Active: "success",
  Suspended: "default",
  Failed: "error",
  Unknown: "default",
};

function Caption({ children }: { children: ReactNode }) {
  return (
    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
      {children}
    </Typography>
  );
}

function AgentIdTile({ env }: { env: AgentRegistryEnvironment }) {
  const status = env.agentIdStatus;
  return (
    <Tile title="Agent ID">
      {status ? (
        <Chip
          size="small"
          variant="outlined"
          color={AGENT_ID_COLORS[status]}
          label={AGENT_ID_LABELS[status]}
        />
      ) : (
        <Gap>No AgentID: it has no credentials of its own</Gap>
      )}
    </Tile>
  );
}

function EndpointSecurityTile({ env }: { env: AgentRegistryEnvironment }) {
  const { apiKey, oauth } = env.inboundSecurity;
  const methods = [apiKey && "API key", oauth && "OAuth"].filter(Boolean) as string[];
  return (
    <Tile title="Endpoint security">
      {methods.length ? (
        <ChipRow labels={methods} />
      ) : (
        <Gap>None</Gap>
      )}
    </Tile>
  );
}

function ModelAccessTile({ env }: { env: AgentRegistryEnvironment }) {
  return (
    <Tile title="LLM providers">
      {env.llmProviders.length ? (
        <>
          <ChipRow labels={env.llmProviders.map((p) => p.displayName)} />
          {/* Guardrails sit on the agent's LLM provider config. */}
          <Caption>
            {env.guardrails.length
              ? `Guardrails: ${env.guardrails.join(", ")}`
              : "No guardrails"}
          </Caption>
        </>
      ) : (
        <Typography variant="body2" color="text.secondary">
          None configured
        </Typography>
      )}
    </Tile>
  );
}

function ToolAccessTile({ env }: { env: AgentRegistryEnvironment }) {
  return (
    <Tile title="MCP servers">
      {env.mcpServers.length ? (
        <Stack spacing={1}>
          {env.mcpServers.map((server) => (
            <Box key={server.id}>
              <Typography variant="body2">{server.displayName}</Typography>
              <Typography variant="caption" color="text.secondary" component="div">
                {server.tools.length ? server.tools.join(", ") : "No tools discovered"}
              </Typography>
            </Box>
          ))}
        </Stack>
      ) : (
        <Typography variant="body2" color="text.secondary">
          None configured
        </Typography>
      )}
    </Tile>
  );
}

function MonitorsTile({ env }: { env: AgentRegistryEnvironment }) {
  return (
    <Tile title="Evaluation monitors">
      {env.monitors.length ? (
        <Stack spacing={0.75}>
          {env.monitors.map((monitor) => (
            <Stack key={monitor.name} direction="row" spacing={1} alignItems="center">
              <Typography variant="body2">{monitor.displayName}</Typography>
              <Chip
                size="small"
                variant="outlined"
                color={MONITOR_COLORS[monitor.status]}
                label={monitor.status}
              />
            </Stack>
          ))}
        </Stack>
      ) : (
        <Gap>None</Gap>
      )}
    </Tile>
  );
}

/**
 * The controls Agent Manager puts around the agent in the environment chosen at the
 * top of the page, read from its existing configuration: its identity, who may call
 * it, which models and tools it may use, and who checks its output.
 */
export function Governance({ env }: { env: AgentRegistryEnvironment }) {
  const half = { xs: 12, md: 6 };
  const third = { xs: 12, md: 4 };
  return (
    <SectionCard title="Governance">
      <Grid container spacing={2}>
        <Grid size={third}>
          <AgentIdTile env={env} />
        </Grid>
        <Grid size={third}>
          <EndpointSecurityTile env={env} />
        </Grid>
        <Grid size={third}>
          <MonitorsTile env={env} />
        </Grid>
        <Grid size={half}>
          <ModelAccessTile env={env} />
        </Grid>
        <Grid size={half}>
          <ToolAccessTile env={env} />
        </Grid>
      </Grid>
    </SectionCard>
  );
}
