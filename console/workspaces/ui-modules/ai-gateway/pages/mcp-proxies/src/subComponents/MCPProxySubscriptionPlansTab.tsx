/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { useEffect, useMemo, useState, type MouseEvent } from "react";
import { Link as RouterLink, generatePath } from "react-router-dom";
import {
  alpha,
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Form,
  Grid,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Check, Layers, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import {
  extractServerErrorMessage,
  useListSubscriptionPlans,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MCPProxy,
  type MCPProxyEndpoint,
  type MCPProxyPolicy,
  type SubscriptionPlan,
} from "@agent-management-platform/types";
import { StateNotice } from "@agent-management-platform/views";
import { useFeatureAccess, useResourceAccess } from "@agent-management-platform/feature-management";
import {
  missingPolicies,
  policyDetails,
  policyLabel,
  requiredPolicies,
  withPoliciesAdded,
} from "./subscriptionPolicies";

/** One card: a plan of the organization, or a handle the proxy offers that isn't listed. */
export type PlanOption = {
  id: string;
  displayName: string;
  description?: string;
  expiryTime?: string;
  status?: SubscriptionPlan["status"];
  conditions?: SubscriptionPlan["conditions"];
};

/** The cards: active plans, plus the plans the proxy already offers, whatever their status. */
export function planOptions(plans: SubscriptionPlan[], offered: string[]): PlanOption[] {
  const byId = new Map(plans.map((p) => [p.id, p]));
  const options: PlanOption[] = plans
    .filter((p) => p.status === "ACTIVE" || offered.includes(p.id))
    .map((p) => ({
      id: p.id,
      displayName: p.displayName,
      description: p.description,
      expiryTime: p.expiryTime,
      status: p.status,
      conditions: p.conditions,
    }));
  for (const id of offered) {
    if (!byId.has(id)) options.push({ id, displayName: id });
  }
  return options;
}

export function samePlans(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((id) => b.includes(id));
}

/** Toggles a plan, keeping the order plans were picked in. */
export function togglePlan(selected: string[], id: string): string[] {
  return selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id];
}

/** "Jan 1, 2030", as the API control plane shows a plan's expiry. */
export const shortDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });

const CHECK_SIZE = 20;
const NAME_LINE_HEIGHT = 28;

/** The plan's name on one line; the whole name on hover, only when it is cut short. */
function PlanName({ name }: { name: string }) {
  const [cut, setCut] = useState(false);
  return (
    <Tooltip describeChild disableHoverListener={!cut} title={name}>
      <Typography
        noWrap
        variant="subtitle1"
        onMouseEnter={(e: MouseEvent<HTMLElement>) =>
          setCut(e.currentTarget.scrollWidth > e.currentTarget.clientWidth)
        }
        sx={{ fontWeight: 600, minWidth: 0 }}
      >
        {name}
      </Typography>
    </Tooltip>
  );
}

/** Up to two lines of description; the whole text on hover, only when it is cut short. */
function PlanDescription({ text }: { text: string }) {
  const [cut, setCut] = useState(false);
  return (
    <Tooltip describeChild disableHoverListener={!cut} title={text} placement="bottom-start">
      <Typography
        variant="body2"
        color="text.secondary"
        onMouseEnter={(e: MouseEvent<HTMLElement>) =>
          setCut(e.currentTarget.scrollHeight > e.currentTarget.clientHeight)
        }
        sx={{
          display: "-webkit-box",
          overflow: "hidden",
          WebkitBoxOrient: "vertical",
          WebkitLineClamp: 2,
          wordBreak: "break-word",
        }}
      >
        {text}
      </Typography>
    </Tooltip>
  );
}

/** The drawn checkbox: a real one can't sit inside the card's button. */
function SelectionSquare({ selected }: { selected: boolean }) {
  return (
    <Box
      sx={{
        alignItems: "center",
        bgcolor: selected ? "primary.main" : "transparent",
        border: 1.5,
        borderColor: selected ? "primary.main" : "text.disabled",
        borderRadius: "6px",
        color: "primary.contrastText",
        display: "flex",
        flexShrink: 0,
        height: CHECK_SIZE,
        justifyContent: "center",
        width: CHECK_SIZE,
      }}
    >
      {selected && <Check size={14} strokeWidth={3} />}
    </Box>
  );
}

/** One plan: the whole card is the checkbox. Name, expiry, then the description. */
function PlanCard({
  option,
  selected,
  onToggle,
}: {
  option: PlanOption;
  selected: boolean;
  onToggle?: () => void;
}) {
  const readOnly = !onToggle;
  return (
    <Form.CardButton
      role="checkbox"
      aria-checked={selected}
      aria-disabled={readOnly}
      aria-label={option.displayName}
      data-testid={`plan-card-${option.id}`}
      onClick={onToggle}
      selected={selected}
      tabIndex={readOnly ? -1 : 0}
      sx={(theme) => ({
        alignItems: "flex-start",
        borderColor: selected ? "primary.main" : "divider",
        display: "flex",
        flexDirection: "column",
        gap: 1.5,
        height: "100%",
        p: 2.5,
        width: "100%",
        ...(selected && { backgroundColor: alpha(theme.palette.primary.main, 0.06) }),
        ...(readOnly && { cursor: "default" }),
      })}
    >
      <Box sx={{ alignItems: "flex-start", display: "flex", gap: 1.5, minWidth: 0, width: "100%" }}>
        <Box sx={{ alignItems: "center", display: "flex", height: NAME_LINE_HEIGHT }}>
          <SelectionSquare selected={selected} />
        </Box>
        <Box sx={{ display: "flex", flex: 1, flexDirection: "column", gap: 0.75, minWidth: 0 }}>
          <Box
            sx={{ alignItems: "center", display: "flex", gap: 1, minHeight: NAME_LINE_HEIGHT }}
          >
            <PlanName name={option.displayName} />
            {option.status === "INACTIVE" && (
              <Chip size="small" variant="outlined" label="Inactive" sx={{ flexShrink: 0 }} />
            )}
          </Box>
          {option.expiryTime && (
            <Chip
              size="small"
              label={`Expires ${shortDate(option.expiryTime)}`}
              sx={{ alignSelf: "flex-start" }}
            />
          )}
        </Box>
      </Box>
      <Box sx={{ minHeight: 40, textAlign: "start", width: "100%" }}>
        {option.description && <PlanDescription text={option.description} />}
      </Box>
    </Form.CardButton>
  );
}

/** The policies, one per line, as `name version`. */
function PolicyBullets({ policies }: { policies: MCPProxyPolicy[] }) {
  return (
    <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2.5 }}>
      {policies.map((p) => (
        <li key={p.name}>
          <Box component="code" sx={{ fontFamily: "monospace" }}>
            {policyLabel(p)}
          </Box>
        </li>
      ))}
    </Box>
  );
}

/** The policies as bordered rows: icon, display name, version chip and a one-line summary. */
function PolicyRows({ policies }: { policies: MCPProxyPolicy[] }) {
  return (
    <Stack component="ul" spacing={1} sx={{ listStyle: "none", m: 0, p: 0 }}>
      {policies.map((p) => {
        const { title, version, summary } = policyDetails(p);
        return (
          <Box
            component="li"
            key={p.name}
            sx={{
              alignItems: "center",
              border: 1,
              borderColor: "divider",
              borderRadius: 1,
              display: "flex",
              gap: 1.5,
              px: 2,
              py: 1.25,
            }}
          >
            <Box sx={{ color: "primary.main", display: "flex", flexShrink: 0 }}>
              <ShieldCheck size={18} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {title}
              </Typography>
              {summary && (
                <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  {summary}
                </Typography>
              )}
            </Box>
            <Chip size="small" variant="outlined" label={version} sx={{ flexShrink: 0 }} />
          </Box>
        );
      })}
    </Stack>
  );
}

export type MCPProxySubscriptionPlansTabProps = {
  proxy: MCPProxy;
  orgName?: string;
  /** Saves the proxy with these plans, and the endpoints when their policies changed. */
  onSave: (subscriptionPlans: string[], endpoints?: MCPProxyEndpoint[]) => Promise<unknown>;
  isSaving: boolean;
};

type AttachPrompt = { plan: string; policies: MCPProxyPolicy[] } | null;

/** The plans an MCP proxy offers, for every endpoint. Shown only with the AI Hub module. */
export function MCPProxySubscriptionPlansTab({
  proxy,
  orgName,
  onSave,
  isSaving,
}: MCPProxySubscriptionPlansTabProps) {
  const { canWrite } = useResourceAccess("mcp_server");
  const canReadPlans = useFeatureAccess("subscription_plan_read");
  const offered = useMemo(() => proxy.subscriptionPlans ?? [], [proxy.subscriptionPlans]);
  const savedEndpoints = useMemo(() => proxy.endpoints ?? [], [proxy.endpoints]);
  const [selected, setSelected] = useState<string[]>(offered);
  const [endpoints, setEndpoints] = useState<MCPProxyEndpoint[]>(savedEndpoints);
  const [prompt, setPrompt] = useState<AttachPrompt>(null);
  const [saveError, setSaveError] = useState<string>();
  useEffect(() => setSelected(offered), [offered]);
  useEffect(() => setEndpoints(savedEndpoints), [savedEndpoints]);

  const { data, isLoading, error } = useListSubscriptionPlans(
    { orgName: canReadPlans ? orgName : undefined },
    { limit: 100 },
  );
  const options = useMemo(() => planOptions(data?.list ?? [], offered), [data, offered]);
  const byId = useMemo(() => new Map(options.map((o) => [o.id, o])), [options]);
  const plansOf = (ids: string[]) =>
    ids.flatMap((id) => byId.get(id) ?? []).map((o) => ({ conditions: o.conditions ?? [] }));

  const editable = canWrite && canReadPlans && !proxy.projectName;
  const shown = editable ? options : options.filter((o) => offered.includes(o.id));
  const policiesChanged = endpoints !== savedEndpoints;
  const dirty = !samePlans(selected, offered) || policiesChanged;
  const missing = data ? missingPolicies(requiredPolicies(plansOf(selected)), endpoints) : [];

  const attachMissing = (policies: MCPProxyPolicy[]) =>
    setEndpoints((eps) =>
      eps.map((ep) => ({ ...ep, policies: withPoliciesAdded(ep.policies ?? [], policies) })),
    );

  const toggle = (option: PlanOption) => {
    const next = togglePlan(selected, option.id);
    setSelected(next);
    if (next.length < selected.length) return;
    // A newly picked plan offers the policies it needs that some endpoint still lacks.
    const needed = missingPolicies(requiredPolicies(plansOf([option.id])), endpoints);
    if (needed.length > 0) setPrompt({ plan: option.displayName, policies: needed });
  };

  const discard = () => {
    setSelected(offered);
    setEndpoints(savedEndpoints);
    setSaveError(undefined);
  };
  const save = async () => {
    setSaveError(undefined);
    try {
      await onSave(selected, policiesChanged ? endpoints : undefined);
    } catch (e) {
      setSaveError(
        extractServerErrorMessage(e) ?? "Couldn't save the subscription plans. Try again.",
      );
    }
  };
  const plansHref = orgName
    ? generatePath(absoluteRouteMap.children.org.children.subscriptionPlans.path, {
        orgId: orgName,
      })
    : undefined;

  return (
    <Stack spacing={2.5} data-testid="mcp-proxy-subscription-plans">
      <Typography variant="body2">
        Plans agents can subscribe to on this MCP server, across all its endpoints.
      </Typography>

      {/* Hidden while the attach dialog is open; shown once it is skipped. */}
      {missing.length > 0 && prompt === null && (
        <Alert
          severity="warning"
          action={
            editable ? (
              <Button
                color="inherit"
                size="small"
                onClick={() => setPrompt({ plan: "The selected plans", policies: missing })}
              >
                Attach
              </Button>
            ) : undefined
          }
        >
          You have not yet attached the following required policies:
          <PolicyBullets policies={missing} />
        </Alert>
      )}
      {error ? (
        <Alert severity="error">
          Couldn&apos;t load the subscription plans. Refresh the page to try again.
        </Alert>
      ) : null}

      {isLoading && canReadPlans ? (
        <Grid container spacing={2}>
          {[0, 1, 2].map((i) => (
            <Grid key={i} size={{ xs: 12, sm: 6, md: 4 }}>
              <Skeleton variant="rounded" height={120} />
            </Grid>
          ))}
        </Grid>
      ) : shown.length === 0 ? (
        <StateNotice
          icon={Layers}
          compact
          title={editable ? "No active subscription plans" : "No plans offered"}
          description={
            editable
              ? "Create a plan, then come back to offer it on this MCP server."
              : "This MCP server doesn't offer any subscription plans."
          }
          actions={
            editable && plansHref ? (
              <Button component={RouterLink} to={plansHref} variant="outlined">
                Go to Subscription Plans
              </Button>
            ) : undefined
          }
        />
      ) : (
        <Grid container spacing={2}>
          {shown.map((option) => (
            <Grid key={option.id} size={{ xs: 12, sm: 6, md: 4 }}>
              <PlanCard
                option={option}
                selected={selected.includes(option.id)}
                onToggle={editable && !isSaving ? () => toggle(option) : undefined}
              />
            </Grid>
          ))}
        </Grid>
      )}

      {saveError && <Alert severity="error">{saveError}</Alert>}
      {editable && (
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button variant="outlined" onClick={discard} disabled={!dirty || isSaving}>
            Discard
          </Button>
          <Button variant="contained" onClick={() => void save()} disabled={!dirty || isSaving}>
            {isSaving ? "Saving..." : "Save"}
          </Button>
        </Stack>
      )}

      <Dialog open={prompt !== null} onClose={() => setPrompt(null)} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ alignItems: "center", display: "flex", gap: 1.5 }}>
          <Box
            sx={(theme) => ({
              alignItems: "center",
              backgroundColor: alpha(theme.palette.primary.main, 0.1),
              borderRadius: 1,
              color: "primary.main",
              display: "flex",
              flexShrink: 0,
              height: 36,
              justifyContent: "center",
              width: 36,
            })}
          >
            <ShieldCheck size={20} />
          </Box>
          Attach required policies?
        </DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <DialogContentText>
              <Box component="strong" sx={{ color: "text.primary", fontWeight: 600 }}>
                {prompt?.plan}
              </Box>{" "}
              needs these policies, which some endpoints of this MCP server don&apos;t have yet.
            </DialogContentText>
            {prompt && <PolicyRows policies={prompt.policies} />}
            <Alert severity="info" variant="outlined">
              Added to every endpoint. Save to deploy.
            </Alert>
          </Stack>
        </DialogContent>
        <DialogActions sx={{ gap: 1, px: 3, pb: 2.5 }}>
          <Button variant="outlined" onClick={() => setPrompt(null)}>
            Skip
          </Button>
          <Button
            variant="contained"
            onClick={() => {
              if (prompt) attachMissing(prompt.policies);
              setPrompt(null);
            }}
          >
            Attach policies
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
