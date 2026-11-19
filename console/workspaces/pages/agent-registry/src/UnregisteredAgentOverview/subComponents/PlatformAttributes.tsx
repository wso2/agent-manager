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

import { Box, Grid, Stack, Typography } from "@wso2/oxygen-ui";
import type { AgentRegistryDiscovery } from "@agent-management-platform/types";
import {
  Field,
  SectionCard,
  ValueText,
} from "../../AgentRegistryOverview/subComponents/SectionCard";
import { ChipRow, Tile } from "../../AgentRegistryOverview/subComponents/Tiles";

// Labels for the attribute keys the discovery connectors write (shadow-agent-registry
// branch). A key not listed here is shown as written.
const ATTRIBUTE_LABELS: Record<string, string> = {
  toolCount: "Tool count",
  region: "Region",
  status: "Status",
  agentVersion: "Agent version",
  resourceId: "Resource ID",
  orchestrationType: "Orchestration type",
  roleArn: "Role ARN",
  project: "Project",
  location: "Location",
  labels: "Labels",
  framework: "Framework",
  serviceAccount: "Service account",
  repositoryUrl: "Repository",
};

// Shown elsewhere: model, tools and MCP servers as tiles, the description under the title.
const NOT_FIELDS = new Set(["model", "tools", "mcpServers", "description"]);

// Long identifiers: monospace, a whole row each.
const WIDE_KEYS = new Set(["resourceId", "roleArn", "serviceAccount", "repositoryUrl"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function records(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? value.filter(isRecord) : [];
}

/** A tool reference ({ type, name }) as "name (type)", or the type alone. */
function toolLabel(tool: Record<string, unknown>): string {
  const type = String(tool.type ?? "");
  return tool.name ? `${String(tool.name)} (${type})` : type;
}

/** Monospace text that wraps anywhere, for IDs and ARNs. */
function Mono({ children }: { children: string }) {
  return (
    <Typography variant="body2" sx={{ fontFamily: "monospace", overflowWrap: "anywhere" }}>
      {children}
    </Typography>
  );
}

/** One attribute that is not a tile: a map as key:value chips, else its text. */
function AttributeValue({ attributeKey, value }: { attributeKey: string; value: unknown }) {
  if (isRecord(value)) {
    const entries = Object.entries(value);
    return entries.length ? (
      <ChipRow labels={entries.map(([k, v]) => `${k}:${String(v)}`)} />
    ) : (
      <ValueText />
    );
  }
  const text = value === undefined || value === null ? "" : String(value);
  return WIDE_KEYS.has(attributeKey) ? <Mono>{text}</Mono> : <ValueText>{text}</ValueText>;
}

/**
 * What the platform reports about the agent, as Shadow Agent Discovery allow-lists it.
 * Model, tools and MCP servers are tiles, like the registered agent's governance; the
 * rest (scope, platform ID, region, role…) are fields. Which keys appear depends on
 * the platform.
 */
export function PlatformAttributes({ discovery }: { discovery: AgentRegistryDiscovery }) {
  const { attributes } = discovery;
  const tools = records(attributes.tools);
  const mcpServers = records(attributes.mcpServers);
  const model = typeof attributes.model === "string" ? attributes.model : undefined;
  const otherAttributes = Object.entries(attributes).filter(([key]) => !NOT_FIELDS.has(key));
  // The tiles present share the row.
  const tileCount = [!!model, tools.length > 0, mcpServers.length > 0].filter(Boolean).length;
  const tileSize = { xs: 12, md: 12 / Math.max(tileCount, 1) };

  return (
    <SectionCard title="Platform details">
      <Stack spacing={2.5}>
        {/* Only what the platform reported, as on the discovery page: no empty tiles. */}
        {tileCount > 0 && (
          <Grid container spacing={2}>
            {model && (
              <Grid size={tileSize}>
                <Tile title="Model">
                  <ValueText>{model}</ValueText>
                </Tile>
              </Grid>
            )}
            {tools.length > 0 && (
              <Grid size={tileSize}>
                <Tile title="Tools">
                  <ChipRow labels={tools.map(toolLabel)} />
                </Tile>
              </Grid>
            )}
            {mcpServers.length > 0 && (
              <Grid size={tileSize}>
                <Tile title="MCP servers">
                  <Stack spacing={1}>
                    {mcpServers.map((server, i) => (
                      <Box key={`${i}-${String(server.label)}`}>
                        <Typography variant="body2">{String(server.label ?? "")}</Typography>
                        {server.origin ? (
                          <Typography variant="caption" color="text.secondary" component="div">
                            {String(server.origin)}
                          </Typography>
                        ) : null}
                      </Box>
                    ))}
                  </Stack>
                </Tile>
              </Grid>
            )}
          </Grid>
        )}
        <Grid container spacing={2.5}>
          <Grid size={{ xs: 12, md: 6 }}>
            <Field label="Scope">
              <Mono>{discovery.scope}</Mono>
            </Field>
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <Field label="Platform ID">
              <Mono>{discovery.externalId}</Mono>
            </Field>
          </Grid>
          {otherAttributes.map(([key, value]) => (
            <Grid key={key} size={WIDE_KEYS.has(key) ? { xs: 12 } : { xs: 12, sm: 6, lg: 3 }}>
              <Field label={ATTRIBUTE_LABELS[key] ?? key}>
                <AttributeValue attributeKey={key} value={value} />
              </Field>
            </Grid>
          ))}
        </Grid>
      </Stack>
    </SectionCard>
  );
}
