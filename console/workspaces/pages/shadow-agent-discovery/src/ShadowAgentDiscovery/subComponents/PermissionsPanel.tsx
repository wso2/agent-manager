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

import { useMemo, useState } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Card,
  CardContent,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  ChevronDown,
  Copy,
  ExternalLink,
  Eye,
  KeyRound,
  ShieldCheck,
  Wrench,
} from "@wso2/oxygen-ui-icons-react";
import type { ShadowPermissionSpec, ShadowProviderSpec } from "@agent-management-platform/types";

/** A policy document, pretty-printed, with a copy button. */
export function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const pretty = useMemo(() => {
    try {
      return JSON.stringify(JSON.parse(text), null, 2);
    } catch {
      return text;
    }
  }, [text]);
  return (
    <Box
      sx={{
        position: "relative",
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        bgcolor: "action.hover",
      }}
    >
      <Box
        component="pre"
        sx={{ m: 0, p: 1.5, pr: 5, overflowX: "auto", fontSize: 12, maxHeight: 260 }}
      >
        {pretty}
      </Box>
      <Tooltip title={copied ? "Copied" : "Copy"}>
        <IconButton
          size="small"
          aria-label="Copy policy"
          sx={{ position: "absolute", top: 4, right: 4 }}
          onClick={() =>
            void navigator.clipboard
              ?.writeText(pretty)
              .then(() => setCopied(true))
              .catch(() => setCopied(false))
          }
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}
        </IconButton>
      </Tooltip>
    </Box>
  );
}

function GroupTitle({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.75 }}>
      <Box sx={{ color: "text.secondary", display: "flex" }}>{icon}</Box>
      <Typography variant="body2" fontWeight={600}>
        {children}
      </Typography>
    </Stack>
  );
}

/** What Agent Manager reads: a checked list. */
function ReadsList({ items }: { items: string[] }) {
  return (
    <Stack spacing={0.75}>
      {items.map((it) => (
        <Stack key={it} direction="row" spacing={1} alignItems="flex-start">
          <Box sx={{ color: "success.main", display: "flex", mt: "3px" }} aria-hidden>
            <Check size={14} />
          </Box>
          <Typography variant="body2" color="text.secondary">
            {it}
          </Typography>
        </Stack>
      ))}
    </Stack>
  );
}

/** How to set it up: numbered steps. */
function StepsList({ items }: { items: string[] }) {
  return (
    <Stack component="ol" spacing={0.75} sx={{ m: 0, pl: 0, listStyle: "none" }}>
      {items.map((it, i) => (
        <Stack key={it} component="li" direction="row" spacing={1} alignItems="flex-start">
          <Box
            sx={{
              minWidth: 20,
              height: 20,
              borderRadius: "50%",
              bgcolor: "action.selected",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 11,
              fontWeight: 600,
              mt: "1px",
            }}
          >
            {i + 1}
          </Box>
          <Typography variant="body2" color="text.secondary">
            {it}
          </Typography>
        </Stack>
      ))}
    </Stack>
  );
}

/**
 * One credential or source: a collapsible section with what is read, how to
 * set it up and the policy.
 */
function Section({ p, defaultExpanded }: { p: ShadowPermissionSpec; defaultExpanded: boolean }) {
  return (
    <Accordion
      defaultExpanded={defaultExpanded}
      disableGutters
      variant="outlined"
      sx={{ "&&": { m: 0, borderRadius: 1 }, "&::before": { display: "none" } }}
    >
      <AccordionSummary expandIcon={<ChevronDown size={18} />} sx={{ px: 2 }}>
        <Stack direction="row" spacing={1.5} alignItems="flex-start" sx={{ minWidth: 0 }}>
          <Box sx={{ color: "primary.main", display: "flex", mt: "2px" }} aria-hidden>
            <KeyRound size={18} />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="subtitle2">{p.title}</Typography>
            {p.summary && (
              <Typography variant="caption" color="text.secondary" component="p">
                {p.summary}
              </Typography>
            )}
          </Box>
        </Stack>
      </AccordionSummary>
      <AccordionDetails sx={{ px: 2, pb: 2, pt: 0 }}>
        <Stack spacing={2}>
          {p.reads && p.reads.length > 0 && (
            <Box>
              <GroupTitle icon={<Eye size={16} />}>What Agent Manager reads</GroupTitle>
              <ReadsList items={p.reads} />
            </Box>
          )}
          {p.items.length > 0 && (
            <Box>
              <GroupTitle icon={<Wrench size={16} />}>How to set it up</GroupTitle>
              <StepsList items={p.items} />
            </Box>
          )}
          {p.policy && (
            <Box>
              <GroupTitle icon={<ShieldCheck size={16} />}>Least-privilege policy</GroupTitle>
              <CopyBlock text={p.policy} />
            </Box>
          )}
          {p.note && (
            <Typography variant="caption" color="text.secondary" component="p">
              {p.note}
            </Typography>
          )}
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
}

function linkLabel(url: string): string {
  try {
    const u = new URL(url);
    return `${u.hostname}${u.pathname === "/" ? "" : u.pathname}`;
  } catch {
    return url;
  }
}

/**
 * What to give Agent Manager for the platform and what it does with it,
 * shown next to the form: a read-only note, one collapsible section per
 * credential or source, and the official documentation.
 */
export function PermissionsPanel({ spec }: { spec: ShadowProviderSpec }) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="subtitle1">How {spec.displayName} is used</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          What to give Agent Manager, and what it does with it.
        </Typography>
        <Stack spacing={1.5}>
          <Stack
            direction="row"
            spacing={1.5}
            alignItems="flex-start"
            sx={{ bgcolor: "action.hover", borderRadius: 1, p: 1.5 }}
          >
            <Box sx={{ color: "success.main", display: "flex", mt: "2px" }} aria-hidden>
              <ShieldCheck size={18} />
            </Box>
            <Box>
              <Typography variant="body2" fontWeight={600}>
                Read-only, and nothing sensitive is stored
              </Typography>
              <Typography variant="caption" color="text.secondary" component="p">
                Every call is read-only. Prompts, completions, tool inputs and outputs and agent
                instructions are never stored or logged; the credential is stored encrypted and is
                never shown again.
              </Typography>
            </Box>
          </Stack>
          {spec.permissions.map((p, i) => (
            <Section key={p.title} p={p} defaultExpanded={i === 0} />
          ))}
          {spec.docs.length > 0 && (
            <Box sx={{ pt: 0.5 }}>
              <Typography variant="caption" color="text.secondary" component="p" sx={{ mb: 0.5 }}>
                Official documentation
              </Typography>
              <Stack spacing={0.5}>
                {spec.docs.map((d) => (
                  <Stack
                    key={d}
                    direction="row"
                    spacing={0.75}
                    alignItems="center"
                    sx={{ minWidth: 0 }}
                  >
                    <Typography variant="body2" sx={{ wordBreak: "break-all", minWidth: 0 }}>
                      <a href={d} target="_blank" rel="noreferrer noopener">
                        {linkLabel(d)}
                      </a>
                    </Typography>
                    <ExternalLink size={12} />
                  </Stack>
                ))}
              </Stack>
            </Box>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

export default PermissionsPanel;
