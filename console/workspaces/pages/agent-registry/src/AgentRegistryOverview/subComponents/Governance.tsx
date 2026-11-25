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

import { Fragment, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Box, Chip, Divider, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { CircleCheck, TriangleAlert } from "@wso2/oxygen-ui-icons-react";
import type {
  AgentRegistryEnvironmentDetail,
  AgentThunderStatus,
  MonitorStatus,
} from "@agent-management-platform/types";
import { SectionCard } from "../../AgentPage/AgentPageParts";

// The Governance card is one aligned list: a narrow label column (Identity, LLMs, MCP
// servers, Monitors) and the values next to it. Each item takes a line, with a muted second
// line for its guardrails or tools. A gap worth noticing (no guardrails, no monitors, an open
// endpoint) carries a warning icon; a control in place carries a check.

/** One line of a value: text after an optional check or warning icon. */
function Line({
  tone,
  muted,
  children,
}: {
  tone?: "ok" | "gap";
  muted?: boolean;
  children: ReactNode;
}) {
  return (
    <Stack direction="row" spacing={0.75} alignItems="flex-start">
      {tone && (
        <Box
          component="span"
          aria-hidden
          sx={{
            display: "inline-flex",
            pt: "3px",
            color: tone === "ok" ? "success.main" : "warning.main",
          }}
        >
          {tone === "ok" ? <CircleCheck size={14} /> : <TriangleAlert size={14} />}
        </Box>
      )}
      <Typography
        variant="body2"
        color={muted ? "text.secondary" : "text.primary"}
        component="div"
        sx={{ minWidth: 0, wordBreak: "break-word" }}
      >
        {children}
      </Typography>
    </Stack>
  );
}

/** Space between chips, in px (theme spacing 0.5). */
const CHIP_GAP = 4;

/**
 * How many chips fit on one line of the given width, leaving room for the "+N" chip when
 * some do not fit. At least one chip shows, so a single long name is cut, not hidden.
 */
export function chipsThatFit(
  chipWidths: number[],
  moreChipWidth: number,
  available: number,
): number {
  const total = chipWidths.reduce((sum, w) => sum + w, 0) + CHIP_GAP * (chipWidths.length - 1);
  if (total <= available) return chipWidths.length;
  let used = moreChipWidth;
  let fit = 0;
  for (const width of chipWidths) {
    if (used + CHIP_GAP + width > available) break;
    used += CHIP_GAP + width;
    fit += 1;
  }
  return Math.max(fit, 1);
}

/**
 * A label and its items as chips on one line: as many chips as the line has room for,
 * then a "+N" chip for the rest. "+N" lists the rest in its tooltip and, when clicked,
 * shows every chip, wrapping; "Show less" goes back.
 */
function ChipLine({ label, items }: { label: string; items: string[] }) {
  const lineRef = useRef<HTMLDivElement>(null);
  const measureRef = useRef<HTMLDivElement>(null);
  const [shown, setShown] = useState(items.length);
  const [expanded, setExpanded] = useState(false);

  // Measure every chip off screen, then fit them to the line's width, again on resize.
  useLayoutEffect(() => {
    const line = lineRef.current;
    const measure = measureRef.current;
    if (!line || !measure) return undefined;
    const fit = () => {
      const chips = Array.from(measure.children) as HTMLElement[];
      const moreChip = chips.pop();
      setShown(
        chipsThatFit(
          chips.map((chip) => chip.offsetWidth),
          moreChip?.offsetWidth ?? 0,
          line.clientWidth,
        ),
      );
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(line);
    return () => observer.disconnect();
  }, [items]);

  const hidden = items.slice(shown);
  const visible = expanded ? items : items.slice(0, shown);
  const chip = (text: string) => (
    <Chip key={text} size="small" variant="outlined" label={text} sx={{ maxWidth: "100%" }} />
  );

  return (
    <Stack direction="row" spacing={1} alignItems="flex-start" sx={{ minWidth: 0 }}>
      <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0, pt: "2px" }}>
        {label}:
      </Typography>
      <Box ref={lineRef} sx={{ position: "relative", flex: 1, minWidth: 0 }}>
        <Stack
          direction="row"
          spacing={0.5}
          useFlexGap
          flexWrap={expanded ? "wrap" : "nowrap"}
          sx={{ overflow: "hidden" }}
        >
          {visible.map(chip)}
          {!expanded && hidden.length > 0 && (
            <Tooltip title={hidden.join(", ")}>
              <Chip
                size="small"
                label={`+${hidden.length}`}
                onClick={() => setExpanded(true)}
                aria-label={`Show ${hidden.length} more`}
              />
            </Tooltip>
          )}
          {expanded && hidden.length > 0 && (
            <Chip size="small" label="Show less" onClick={() => setExpanded(false)} />
          )}
        </Stack>
        {/* Every chip, and a "+N" sample, laid out unseen to measure their widths. */}
        <Stack
          ref={measureRef}
          direction="row"
          aria-hidden
          sx={{ position: "absolute", top: 0, left: 0, visibility: "hidden", pointerEvents: "none" }}
        >
          {items.map(chip)}
          <Chip size="small" label={`+${items.length}`} />
        </Stack>
      </Box>
    </Stack>
  );
}

/** One row of the list: its label in the narrow column, its lines next to it. */
function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack direction={{ xs: "column", sm: "row" }} spacing={{ xs: 0.5, sm: 2 }}>
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ width: { sm: 140 }, flexShrink: 0 }}
      >
        {label}
      </Typography>
      <Stack spacing={1} sx={{ minWidth: 0, flex: 1 }}>
        {children}
      </Stack>
    </Stack>
  );
}

const AGENT_ID_TEXT: Record<AgentThunderStatus, string> = {
  completed: "Agent ID issued",
  in_progress: "Agent ID being issued",
  pending: "Agent ID pending",
  failed: "Agent ID could not be issued",
};

/** The callers' credentials, as words: "an API key", "OAuth", "an API key or OAuth". */
function callerCredentials(apiKey: boolean, oauth: boolean): string {
  if (apiKey && oauth) return "an API key or OAuth";
  return apiKey ? "an API key" : "OAuth";
}

/** Its AgentID, and how callers must authenticate to its endpoint. */
function IdentityRow({ env }: { env: AgentRegistryEnvironmentDetail }) {
  const status = env.agentIdStatus;
  const { apiKey, oauth } = env.inboundSecurity;
  return (
    <Row label="Identity">
      {status ? (
        <Line tone={status === "completed" ? "ok" : status === "failed" ? "gap" : undefined}>
          {AGENT_ID_TEXT[status]}
        </Line>
      ) : (
        <Line tone="gap">No Agent ID</Line>
      )}
      {apiKey || oauth ? (
        <Line tone="ok">Callers use {callerCredentials(apiKey, oauth)}</Line>
      ) : (
        <Line tone="gap">Endpoint open: callers need no credentials</Line>
      )}
    </Row>
  );
}

/** Each LLM configuration, with its provider when named differently, and its guardrails. */
function LlmsRow({ env }: { env: AgentRegistryEnvironmentDetail }) {
  return (
    <Row label="LLMs">
      {env.llmConfigurations.length === 0 && <Line muted>None configured</Line>}
      {env.llmConfigurations.map((config) => (
        <Box key={config.name}>
          <Line>
            {config.name}
            {config.provider.displayName !== config.name && (
              <Typography component="span" variant="body2" color="text.secondary">
                {` (${config.provider.displayName})`}
              </Typography>
            )}
          </Line>
          {config.guardrails.length ? (
            <ChipLine label="Guardrails" items={config.guardrails} />
          ) : (
            <Line tone="gap" muted>
              No guardrails
            </Line>
          )}
        </Box>
      ))}
    </Row>
  );
}

/** Each MCP server, with the tools discovered on it. */
function McpServersRow({ env }: { env: AgentRegistryEnvironmentDetail }) {
  return (
    <Row label="MCP servers">
      {env.mcpServers.length === 0 && <Line muted>None configured</Line>}
      {env.mcpServers.map((server) => (
        <Box key={server.id}>
          <Line>{server.displayName}</Line>
          {server.tools.length ? (
            <ChipLine label="Tools" items={server.tools} />
          ) : (
            <Line muted>No tools discovered</Line>
          )}
        </Box>
      ))}
    </Row>
  );
}

/** The colour of a monitor's status dot. */
const MONITOR_DOT_COLORS: Record<MonitorStatus, string> = {
  Active: "success.main",
  Suspended: "text.disabled",
  Failed: "error.main",
  Unknown: "text.disabled",
};

/** The evaluation monitors on it, each with its status after a coloured dot. */
function MonitorsRow({ env }: { env: AgentRegistryEnvironmentDetail }) {
  return (
    <Row label="Monitors">
      {env.monitors.length === 0 ? (
        <Line tone="gap">None</Line>
      ) : (
        // Two columns, names then statuses, so the statuses line up whatever the names' length.
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "minmax(0, max-content) max-content",
            columnGap: 4,
            rowGap: 1,
            alignItems: "center",
          }}
        >
          {env.monitors.map((monitor) => (
            <Fragment key={monitor.name}>
              <Typography variant="body2" sx={{ minWidth: 0, wordBreak: "break-word" }}>
                {monitor.displayName}
              </Typography>
              <Stack direction="row" spacing={0.75} alignItems="center">
                <Box
                  component="span"
                  aria-hidden
                  sx={{
                    width: 8,
                    height: 8,
                    borderRadius: "50%",
                    bgcolor: MONITOR_DOT_COLORS[monitor.status],
                  }}
                />
                <Typography component="span" variant="body2" color="text.secondary">
                  {monitor.status}
                </Typography>
              </Stack>
            </Fragment>
          ))}
        </Box>
      )}
    </Row>
  );
}

/**
 * The controls around the agent in the environment chosen at the top of the page, read
 * from its existing configuration: its identity and who may call it, the LLMs and MCP
 * servers it may use, and what evaluates its output.
 */
export function GovernanceCard({ env }: { env: AgentRegistryEnvironmentDetail }) {
  return (
    <SectionCard title="Governance">
      <Stack spacing={2} divider={<Divider flexItem />}>
        <IdentityRow env={env} />
        <LlmsRow env={env} />
        <McpServersRow env={env} />
        <MonitorsRow env={env} />
      </Stack>
    </SectionCard>
  );
}
