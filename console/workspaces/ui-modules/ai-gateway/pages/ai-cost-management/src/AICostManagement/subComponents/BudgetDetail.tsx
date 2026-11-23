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
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  formatRelativeTime,
  Link,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Bell,
  BrainCircuit,
  CalendarDays,
  Clock,
  Coins,
  Filter,
  Flag,
  Layers,
  Mail,
  Pencil,
  Power,
  ServerCrash,
  Target,
  Trash2,
  Users,
  Wallet,
  Zap,
  SearchX,
  Tag,
} from "@wso2/oxygen-ui-icons-react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import {
  useBudgetUsage,
  useGetBudget,
  useUpdateBudget,
} from "@agent-management-platform/api-client";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import { LabelChips } from "@agent-management-platform/shared-component";
import {
  PageLayout,
  PageMetaItem,
  StateNotice,
} from "@agent-management-platform/views";
import {
  BUDGET_PROJECT_ANNOTATION,
  type Budget,
  type BudgetFilterType,
  type BudgetPeriod,
} from "@agent-management-platform/types";
import { amount, type CostNames } from "../costFormat";
import { isBudgetNotFound, wordLabels } from "../budgetForm";
import { useConfirmDeleteBudget } from "./useConfirmDeleteBudget";
import { PERIOD_LABELS, inSentence } from "./budgetLabels";
import { separateKind } from "./BudgetsTab";
import { useEntityNames } from "../useEntityNames";

interface BudgetDetailProps {
  orgName?: string;
  budgetName?: string;
  /** The Budgets list this page returns to. */
  backHref: string;
  /** The budget's edit page. */
  editHref: string;
  /** A project's view: budgets are managed at organization level only. */
  readOnly?: boolean;
  /** The Requests charged $0 page. */
  unpricedHref?: string;
}

/** The Requests charged $0 page narrowed to what the budget names: its agent, else its
 * project, else its LLM provider. */
function unpricedLink(
  href: string,
  budget: Budget | undefined,
  projectView: boolean,
): string {
  const filters = budget?.spec.filters ?? [];
  const agent = filters.find((f) => f.type === "agent" && f.project);
  const project = filters.find((f) => f.type === "project");
  const provider = filters.find((f) => f.type === "llmProvider")?.id;
  const [groupBy, pick] = agent
    ? ["agent", `${agent.project}/${agent.id}`]
    : project && !projectView // a project's page is that project already
      ? ["project", project.id ?? ""]
      : provider
        ? ["llmProvider", provider]
        : [];
  if (!groupBy || !pick) return href;
  return `${href}?${new URLSearchParams({ groupBy, pick }).toString()}`;
}

const KIND_LABEL: Record<BudgetFilterType, string> = {
  llmProvider: "LLM provider",
  project: "Project",
  agent: "Agent",
  apiKey: "API key",
  attribute: "Custom attribute",
};

/** USD with cents, for limits and spend on this page. */
function usd(value: number): string {
  return value.toLocaleString("en-US", { style: "currency", currency: "USD" });
}

function utcDate(d: Date): string {
  return d.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  });
}

/** The current period's first day and the day it resets (UTC). */
function periodSpan(
  start: string,
  period: BudgetPeriod,
): { from: string; resets: string } | null {
  const d = new Date(`${start}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return null;
  const from = utcDate(d);
  if (period === "day") d.setUTCDate(d.getUTCDate() + 1);
  else if (period === "week") d.setUTCDate(d.getUTCDate() + 7);
  else d.setUTCMonth(d.getUTCMonth() + 1);
  return { from, resets: utcDate(d) };
}

/** A rounded square holding an icon, as the reference design's section and row icons. */
function IconTile({
  children,
  accent,
}: {
  children: ReactNode;
  accent?: boolean;
}) {
  return (
    <Box
      sx={{
        width: 44,
        height: 44,
        borderRadius: 1.5,
        display: "grid",
        placeItems: "center",
        flexShrink: 0,
        bgcolor: "action.hover",
        color: accent ? "primary.main" : "text.secondary",
      }}
    >
      {children}
    </Box>
  );
}

/** Block reads as a warning; Allow as a go-ahead. */
function ActionChip({ action }: { action: "block" | "allow" }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color={action === "block" ? "warning" : "default"}
      label={action === "block" ? "Block" : "Allow"}
    />
  );
}

function actionSentence(
  action: "block" | "allow",
  limit: number,
  blockAt: number,
): string {
  return action === "block"
    ? `Blocks at ${usd((limit * blockAt) / 100)} (${blockAt}%).`
    : "Only warns.";
}

/** A labelled figure beside an accent icon, inside a card. */
function Figure({
  icon,
  label,
  value,
  hint,
}: {
  icon: ReactNode;
  label: string;
  value: ReactNode;
  hint?: string;
}) {
  return (
    <Stack direction="row" spacing={2} alignItems="center">
      <Box sx={{ color: "primary.main", display: "flex" }}>{icon}</Box>
      <Box>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography variant="h6" sx={{ fontVariantNumeric: "tabular-nums" }}>
          {value}
        </Typography>
        {hint && (
          <Typography variant="caption" color="text.secondary">
            {hint}
          </Typography>
        )}
      </Box>
    </Stack>
  );
}

/** This period's spend against the limit. */
function ThisPeriod({ budget }: { budget: Budget }) {
  if (separateKind(budget)) return <ThisPeriodEach budget={budget} />;
  const { limit: lim, warnings } = budget.spec;
  const limit = amount(lim.amount);
  const period = budget.status?.currentPeriod;
  const spent = amount(period?.spent);
  const share = limit > 0 ? (spent / limit) * 100 : 0;
  const blockAt = lim.action === "block" ? (lim.blockAt ?? 100) : undefined;
  const span = period ? periodSpan(period.periodStart, lim.period) : null;
  const fill =
    share >= (blockAt ?? 100)
      ? "error.main"
      : share >= (blockAt ?? 100) * 0.8
        ? "warning.main"
        : "primary.main";
  // Ticks where warnings fire and where blocking starts.
  // Ticks for thresholds within the limit; ones above 100% are listed under the bar,
  // since the bar ends at the limit.
  const thresholds = warnings?.thresholds ?? [];
  const beyond = thresholds.filter((t) => t > 100).sort((a, b) => a - b);
  const marks = [
    ...thresholds
      .filter((t) => t <= 100 && t !== blockAt)
      .map((t) => ({ at: t, block: false })),
    ...(blockAt !== undefined && blockAt < 100
      ? [{ at: blockAt, block: true }]
      : []),
  ];
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 3 }}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="subtitle1" fontWeight={600}>
            This period
          </Typography>
          {span && (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              color="text.secondary"
            >
              <CalendarDays size={16} />
              <Typography variant="body2">
                {PERIOD_LABELS[lim.period]} · {span.from} – {span.resets} (UTC)
              </Typography>
            </Stack>
          )}
        </Stack>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="baseline"
          sx={{ mt: 2 }}
        >
          <Stack
            direction="row"
            spacing={1}
            alignItems="baseline"
            flexWrap="wrap"
          >
            <Typography
              fontWeight={700}
              sx={{
                fontSize: { xs: 32, md: 40 },
                lineHeight: 1.1,
                fontVariantNumeric: "tabular-nums",
              }}
            >
              {usd(spent)}
            </Typography>
            <Typography variant="h6" color="text.secondary" fontWeight={400}>
              of {usd(limit)} used
            </Typography>
          </Stack>
          <Typography
            variant="h5"
            fontWeight={600}
            sx={{ fontVariantNumeric: "tabular-nums" }}
          >
            {Math.round(share)}%
          </Typography>
        </Stack>
        <Box sx={{ position: "relative", mt: 2 }}>
          <Box
            role="meter"
            aria-label="Share of the limit spent"
            aria-valuenow={Math.round(share)}
            aria-valuemin={0}
            aria-valuemax={100}
            sx={{
              height: 14,
              borderRadius: 7,
              bgcolor: "action.hover",
              overflow: "hidden",
            }}
          >
            <Box
              sx={{
                width: `${Math.min(share, 100)}%`,
                height: "100%",
                bgcolor: fill,
                borderRadius: 7,
              }}
            />
          </Box>
          {marks.map((m) => (
            <Tooltip
              key={`${m.block}-${m.at}`}
              title={m.block ? `Blocks at ${m.at}%` : `Warns at ${m.at}%`}
            >
              <Box
                sx={{
                  position: "absolute",
                  left: `${m.at}%`,
                  top: -3,
                  width: 3,
                  height: 20,
                  borderRadius: 1,
                  transform: "translateX(-50%)",
                  bgcolor: m.block ? "error.main" : "text.secondary",
                }}
              />
            </Tooltip>
          ))}
        </Box>
        {beyond.length > 0 && (
          <Typography
            variant="caption"
            color="text.secondary"
            component="div"
            sx={{ mt: 1.5 }}
          >
            Also warns at{" "}
            {beyond.length === 1
              ? `${beyond[0]}%`
              : `${beyond
                  .slice(0, -1)
                  .map((t) => `${t}%`)
                  .join(", ")} and ${beyond[beyond.length - 1]}%`}
            , after the limit is exceeded.
          </Typography>
        )}
        <Divider sx={{ my: 2.5 }} />
        <Stack
          direction={{ xs: "column", sm: "row" }}
          divider={<Divider orientation="vertical" flexItem />}
          spacing={3}
        >
          <Box sx={{ flex: 1 }}>
            <Figure
              icon={<Coins size={28} />}
              {...remainingFigure(
                limit,
                spent,
                blockAt,
                budget.spec.status === "inactive",
              )}
            />
          </Box>
          <Box sx={{ flex: 1 }}>
            <Figure
              icon={<CalendarDays size={28} />}
              label="Resets"
              value={span?.resets ?? "—"}
              hint="UTC"
            />
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}

const PERIOD_PER: Record<string, string> = {
  day: "per day",
  week: "per week",
  month: "per month",
};

/**
 * This period of a budget counted separately (each agent, each API key, ...): every
 * allowance has the whole limit, so the total is a sum, not a share of it. The list of
 * allowances below shows each one against its own limit.
 */
function ThisPeriodEach({ budget }: { budget: Budget }) {
  const lim = budget.spec.limit;
  const kind = separateKind(budget) ?? "value";
  const period = budget.status?.currentPeriod;
  const span = period ? periodSpan(period.periodStart, lim.period) : null;
  const limit = amount(lim.amount);
  const blockAt = lim.action === "block" ? (lim.blockAt ?? 100) : undefined;
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 3 }}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="subtitle1" fontWeight={600}>
            This period
          </Typography>
          {span && (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              color="text.secondary"
            >
              <CalendarDays size={16} />
              <Typography variant="body2">
                {PERIOD_LABELS[lim.period]} · {span.from} – {span.resets} (UTC)
              </Typography>
            </Stack>
          )}
        </Stack>
        <Typography variant="h6" sx={{ mt: 2 }}>
          {`Each ${kind} can spend up to ${usd(limit)} ${PERIOD_PER[lim.period]}`}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {blockAt !== undefined
            ? `Blocked at ${usd((limit * blockAt) / 100)}.`
            : "Only warns."}
        </Typography>
        <Divider sx={{ my: 2.5 }} />
        <Stack
          direction={{ xs: "column", sm: "row" }}
          divider={<Divider orientation="vertical" flexItem />}
          spacing={3}
        >
          <Box sx={{ flex: 1 }}>
            <Figure
              icon={<Coins size={28} />}
              label="Spent in total"
              value={usd(amount(period?.spent))}
              hint="Across all entries below"
            />
          </Box>
          <Box sx={{ flex: 1 }}>
            <Figure
              icon={<CalendarDays size={28} />}
              label="Resets"
              value={span?.resets ?? "—"}
              hint="UTC"
            />
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}

/**
 * What is left this period: up to the block point for a Block budget (requests stop
 * there, not at the amount), up to the amount for an Allow budget, which can go over.
 */
function remainingFigure(
  limit: number,
  spent: number,
  blockAt?: number,
  inactive = false,
) {
  if (blockAt !== undefined) {
    const point = (limit * blockAt) / 100;
    return {
      label: "Remaining",
      value: usd(Math.max(point - spent, 0)),
      // A disabled budget counts and blocks nothing.
      hint: inactive
        ? "Not enforced while disabled"
        : spent >= point
          ? "Requests are blocked"
          : `Until blocking at ${usd(point)}`,
    };
  }
  return spent > limit
    ? {
        label: "Over the limit",
        value: usd(spent - limit),
        hint: "Allowed; warnings only",
      }
    : { label: "Remaining", value: usd(limit - spent) };
}

/** A small card: an icon tile, a title and its content. */
interface Titled {
  icon: ReactNode;
  title: string;
}

function SideCard({ icon, title, children }: Titled & { children: ReactNode }) {
  return (
    <Card variant="outlined" sx={{ flex: 1 }}>
      <CardContent sx={{ p: 3, "&:last-child": { pb: 3 } }}>
        <Stack direction="row" spacing={2}>
          <IconTile accent>{icon}</IconTile>
          <Box>
            <Typography variant="subtitle1" fontWeight={600}>
              {title}
            </Typography>
            <Box sx={{ mt: 0.75 }}>{children}</Box>
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}

/** A card heading: an icon, a title and a one-line purpose. */
function CardHeading({ icon, title, purpose }: Titled & { purpose: string }) {
  return (
    <Stack direction="row" spacing={2} alignItems="flex-start" sx={{ mb: 2 }}>
      <Box sx={{ color: "text.secondary", display: "flex", mt: 0.25 }}>
        {icon}
      </Box>
      <Box>
        <Typography variant="h6">{title}</Typography>
        <Typography variant="body2" color="text.secondary">
          {purpose}
        </Typography>
      </Box>
    </Stack>
  );
}

/** One line of the scope: what it is, its value in bold, a note below. */
function ScopeRow({
  icon,
  label,
  value,
  note,
  badge,
}: {
  icon: ReactNode;
  label: string;
  value: ReactNode;
  note?: ReactNode;
  badge?: string;
}) {
  return (
    <Stack direction="row" spacing={2} alignItems="center" sx={{ py: 1.75 }}>
      <IconTile>{icon}</IconTile>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography
          variant="body1"
          component="div"
          fontWeight={600}
          sx={{ wordBreak: "break-word" }}
        >
          {value}
        </Typography>
        {note && (
          <Typography variant="body2" color="text.secondary" component="div">
            {note}
          </Typography>
        )}
      </Box>
      {badge && <Chip size="small" variant="outlined" label={badge} />}
    </Stack>
  );
}

/** A filter or exemption as the page names it: display names where known. */
function entityText(
  e: { type: string; id?: string; project?: string; expression?: string },
  names: CostNames,
): string {
  if (e.type === "attribute") return e.expression ?? "";
  if (e.type === "llmProvider")
    return names.llmProvider?.[e.id ?? ""] ?? e.id ?? "";
  if (e.type === "project") return names.project?.[e.id ?? ""] ?? e.id ?? "";
  if (e.type === "agent" && e.id) {
    const agent = e.project
      ? (names.agent?.[`${e.project}/${e.id}`] ?? e.id)
      : e.id;
    return e.project
      ? `${names.project?.[e.project] ?? e.project} / ${agent}`
      : agent;
  }
  if (!e.id)
    return `Any ${inSentence(KIND_LABEL[e.type as keyof typeof KIND_LABEL] ?? e.type)}`;
  return e.id;
}

/** The filters one per line, joined by AND as in the form: "Agent  Any agent". */
function FilterList({
  filters,
  names,
}: {
  filters: {
    type: string;
    id?: string;
    project?: string;
    expression?: string;
  }[];
  names: CostNames;
}) {
  // One chip per filter, type then value; every filter must match.
  return (
    <Stack
      direction="row"
      flexWrap="wrap"
      useFlexGap
      spacing={1}
      sx={{ mt: 0.5 }}
    >
      {filters.map((f, i) => (
        <Chip
          key={i}
          variant="outlined"
          label={
            <Box component="span" sx={{ display: "inline-flex", gap: 0.75 }}>
              <Box
                component="span"
                sx={{ color: "text.secondary", fontWeight: 400 }}
              >
                {KIND_LABEL[f.type as keyof typeof KIND_LABEL] ?? f.type}
              </Box>
              <Box
                component="span"
                sx={{
                  fontWeight: 600,
                  ...(f.type === "attribute"
                    ? {
                        fontFamily: "monospace",
                        whiteSpace: "normal",
                        wordBreak: "break-all",
                      }
                    : {}),
                }}
              >
                {entityText(f, names)}
              </Box>
            </Box>
          }
          sx={{
            height: "auto",
            py: 0.5,
            "& .MuiChip-label": { whiteSpace: "normal" },
          }}
        />
      ))}
    </Stack>
  );
}

function Scope({ budget, orgName }: { budget: Budget; orgName: string }) {
  const { spec, metadata } = budget;
  const names = useEntityNames(orgName);
  const owner = metadata.annotations?.[BUDGET_PROJECT_ANNOTATION];
  const filters = spec.filters ?? [];
  const split = spec.limit.splitBy;
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 3 }}>
        <CardHeading
          icon={<Target size={24} />}
          title="Applies to"
          purpose="What counts and how it's shared."
        />
        <Stack divider={<Divider flexItem />}>
          <ScopeRow
            icon={<Users size={20} />}
            label="Filters"
            value={
              filters.length ? (
                <FilterList filters={filters} names={names} />
              ) : (
                "All LLM usage"
              )
            }
            note={
              filters.length
                ? "Match all filters."
                : owner
                  ? `All usage in project ${names.project?.[owner] ?? owner}`
                  : "Everything in the organization"
            }
          />
          <ScopeRow
            icon={<BrainCircuit size={20} />}
            label="Limit"
            value={
              split
                ? `Separate limit per ${inSentence(KIND_LABEL[split.type])}`
                : "One shared limit"
            }
            note={
              split
                ? split.expression
                  ? `Told apart by ${split.expression}`
                  : `Each ${inSentence(KIND_LABEL[split.type])} gets its own ${usd(amount(spec.limit.amount))} limit`
                : "All matching usage shares one limit"
            }
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

/** A label and its value on one line, after an icon. */
function AlertRow({
  icon,
  label,
  children,
}: {
  icon: ReactNode;
  label: string;
  children: ReactNode;
}) {
  return (
    <Stack
      direction="row"
      spacing={2}
      alignItems="center"
      sx={{ py: 1.75, px: 1 }}
    >
      <Box sx={{ color: "text.secondary", display: "flex" }}>{icon}</Box>
      <Typography
        variant="body1"
        sx={{ width: { xs: 90, sm: 140 }, flexShrink: 0 }}
      >
        {label}
      </Typography>
      <Box sx={{ flex: 1, minWidth: 0, overflowWrap: "anywhere" }}>
        {children}
      </Box>
    </Stack>
  );
}

/** Exemptions, warning thresholds and channels: the wizard's third step. */
function Alerts({ budget, orgName }: { budget: Budget; orgName: string }) {
  const { warnings } = budget.spec;
  const exemptions = budget.spec.exemptions ?? [];
  const names = useEntityNames(orgName);
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 3 }}>
        <CardHeading
          icon={<Bell size={24} />}
          title="Exemptions and alerts"
          purpose="Exclusions and warnings."
        />
        <Stack divider={<Divider flexItem />}>
          <AlertRow icon={<Filter size={22} />} label="Exemptions">
            {exemptions.length ? (
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                {exemptions.map((e, i) => (
                  <Chip
                    key={i}
                    size="small"
                    variant="outlined"
                    label={`${KIND_LABEL[e.type as keyof typeof KIND_LABEL] ?? e.type}: ${entityText(e, names)}`}
                  />
                ))}
              </Stack>
            ) : (
              <Typography variant="body1" fontWeight={500}>
                None
              </Typography>
            )}
          </AlertRow>
          <AlertRow icon={<Flag size={22} />} label="Warn at">
            {warnings?.thresholds.length ? (
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                {warnings.thresholds.map((t) => (
                  <Chip key={t} size="small" label={`${t}%`} />
                ))}
              </Stack>
            ) : (
              <Typography variant="body1" fontWeight={500}>
                No warnings
              </Typography>
            )}
          </AlertRow>
          <AlertRow icon={<Mail size={22} />} label="Notify">
            {warnings?.channels.length ? (
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                {warnings.channels.map((c) => (
                  <Chip key={c} size="small" variant="outlined" label={c} />
                ))}
              </Stack>
            ) : (
              <Typography variant="body1" fontWeight={500}>
                None
              </Typography>
            )}
          </AlertRow>
        </Stack>
      </CardContent>
    </Card>
  );
}

/** Spend per allowance, when the budget's limit is split. */
function Allowances({ budget, orgName }: { budget: Budget; orgName?: string }) {
  const limit = amount(budget.spec.limit.amount);
  const blockAt =
    budget.spec.limit.action === "block"
      ? (budget.spec.limit.blockAt ?? 100)
      : undefined;
  const point = blockAt !== undefined ? (limit * blockAt) / 100 : limit;
  const inactive = budget.spec.status === "inactive";
  const names = useEntityNames(orgName ?? "");
  const usage = useBudgetUsage(
    { orgName, budgetName: budget.metadata.name },
    { limit: 100 },
  );
  const kind = separateKind(budget);
  if (!kind) return null;
  if (usage.error) {
    return (
      <Alert
        severity="error"
        action={
          <Button
            color="inherit"
            size="small"
            onClick={() => void usage.refetch()}
          >
            Retry
          </Button>
        }
      >
        Couldn&apos;t load the spend per allowance.
      </Alert>
    );
  }
  const rows = allowanceRows(budget, usage.data?.allowances ?? [], names);
  return (
    <Card variant="outlined">
      <CardContent sx={{ p: 3 }}>
        <CardHeading
          icon={<Layers size={24} />}
          title="Spend breakdown"
          purpose="Each has its own limit."
        />
        {usage.isLoading ? (
          <Skeleton variant="rounded" height={120} />
        ) : rows.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            Nothing has been spent this period.
          </Typography>
        ) : (
          <Stack divider={<Divider flexItem />}>
            {rows.map((r) => {
              const share = limit > 0 ? (r.spent / limit) * 100 : 0;
              const reached = r.spent >= point;
              const fill = reached
                ? "error.main"
                : share >= (blockAt ?? 100) * 0.8
                  ? "warning.main"
                  : "primary.main";
              return (
                <Stack key={r.key} spacing={0.75} sx={{ py: 1.5 }}>
                  <Stack
                    direction={{ xs: "column", sm: "row" }}
                    justifyContent="space-between"
                    alignItems={{ xs: "flex-start", sm: "baseline" }}
                    spacing={{ xs: 0.25, sm: 2 }}
                  >
                    <Typography
                      variant="body2"
                      fontWeight={600}
                      sx={{ overflowWrap: "anywhere", minWidth: 0 }}
                    >
                      {r.label}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        fontVariantNumeric: "tabular-nums",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {`${usd(r.spent)} of ${usd(limit)} · ${Math.round(share)}%`}
                    </Typography>
                  </Stack>
                  <Box
                    sx={{
                      height: 8,
                      borderRadius: 4,
                      bgcolor: "action.hover",
                      overflow: "hidden",
                    }}
                  >
                    <Box
                      sx={{
                        width: `${Math.min(share, 100)}%`,
                        height: "100%",
                        bgcolor: fill,
                        borderRadius: 4,
                      }}
                    />
                  </Box>
                  <Typography
                    variant="caption"
                    color={
                      reached && !inactive ? "error.main" : "text.secondary"
                    }
                  >
                    {inactive
                      ? "Not enforced while disabled"
                      : reached
                        ? blockAt !== undefined
                          ? "Requests are blocked"
                          : `Over the limit by ${usd(r.spent - limit)}`
                        : blockAt !== undefined
                          ? `${usd(point - r.spent)} until blocking`
                          : `${usd(limit - r.spent)} remaining`}
                  </Typography>
                </Stack>
              );
            })}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}

interface AllowanceRow {
  key: string;
  label: string;
  spent: number;
  unpriced: number;
}

/**
 * Every allowance of a budget counted separately: those with spend this period and, for
 * agents and projects, the ones in scope that have not spent yet (they can be listed;
 * API keys, users and attribute values cannot). Display names; highest share first.
 */
function allowanceRows(
  budget: Budget,
  usage: {
    allowance: Record<string, string>;
    spent: string;
    unpricedRequests: number;
  }[],
  names: CostNames,
): AllowanceRow[] {
  const filters = budget.spec.filters ?? [];
  const exemptions = budget.spec.exemptions ?? [];
  const label = (a: Record<string, string>) => {
    if (a.agent) {
      const project = a.project ?? "";
      const agent = names.agent?.[`${project}/${a.agent}`] ?? a.agent;
      return project
        ? `${names.project?.[project] ?? project} / ${agent}`
        : agent;
    }
    if (a.project) return names.project?.[a.project] ?? a.project;
    if (a.llmProvider)
      return names.llmProvider?.[a.llmProvider] ?? a.llmProvider;
    return Object.values(a).join(" · ") || "All traffic";
  };
  const rows = new Map<string, AllowanceRow>();
  for (const u of usage) {
    const key = JSON.stringify(u.allowance);
    rows.set(key, {
      key,
      label: label(u.allowance),
      spent: amount(u.spent),
      unpriced: u.unpricedRequests,
    });
  }
  const zero = (a: Record<string, string>) => {
    const key = JSON.stringify(a);
    if (!rows.has(key))
      rows.set(key, { key, label: label(a), spent: 0, unpriced: 0 });
  };
  const owner = budget.metadata.annotations?.[BUDGET_PROJECT_ANNOTATION];
  const sharedProject =
    filters.find((f) => f.type === "project" && f.id)?.id ??
    filters.find((f) => f.type === "agent" && f.project)?.project ??
    owner;
  const separate = (type: string) => budget.spec.limit.splitBy?.type === type;
  if (separate("agent")) {
    for (const key of Object.keys(names.agent ?? {})) {
      const [project, agent] = key.split("/");
      if (sharedProject && project !== sharedProject) continue;
      if (
        exemptions.some(
          (e) => e.type === "agent" && e.id === agent && e.project === project,
        )
      )
        continue;
      if (exemptions.some((e) => e.type === "project" && e.id === project))
        continue;
      // The gateway reports an agent as its project and agent (allowance values).
      zero({ agent, project });
    }
  } else if (separate("project")) {
    for (const project of Object.keys(names.project ?? {})) {
      if (exemptions.some((e) => e.type === "project" && e.id === project))
        continue;
      zero({ project });
    }
  }
  return [...rows.values()].sort(
    (a, b) => b.spent - a.spent || a.label.localeCompare(b.label),
  );
}

/** A budget: this period's spend, its enforcement, scope and limit; Edit, Disable, Delete. */
export function BudgetDetail({
  orgName,
  budgetName,
  backHref,
  editHref,
  readOnly,
  unpricedHref,
}: BudgetDetailProps) {
  const navigate = useNavigate();
  const { hasAccess } = useFeatureManagement();
  const canUpdate = !readOnly && hasAccess("budget_update");
  const canDelete = !readOnly && hasAccess("budget_delete");
  const canReadCost = hasAccess("ai_cost_read");
  const {
    data: budget,
    isLoading,
    error,
    refetch,
  } = useGetBudget({ orgName, budgetName });
  const notFound = isBudgetNotFound(error);
  const { mutate: update, isPending: isUpdating } = useUpdateBudget({
    orgName,
    budgetName,
  });
  const { confirmDelete: confirmBudgetDelete, isDeleting } =
    useConfirmDeleteBudget(orgName);

  const fromGateway = budget?.status?.origin === "gateway_api";
  const inactive = budget?.spec.status === "inactive";
  const name = notFound
    ? "Budget not found"
    : (budget?.spec.displayName ?? budget?.metadata.name ?? budgetName ?? "");
  const owner = budget?.metadata.annotations?.[BUDGET_PROJECT_ANNOTATION];
  const ownerName = useEntityNames(orgName ?? "").project?.[owner ?? ""];
  const updatedAt = budget?.status?.updatedAt ?? budget?.status?.createdAt;

  const toggle = () => {
    if (!budget) return;
    // The kind goes back whole with the status flipped; the server sets `status`.
    const { apiVersion, kind, metadata, spec } = budget;
    update({
      apiVersion,
      kind,
      metadata,
      spec: { ...spec, status: inactive ? "active" : "inactive" },
    });
  };

  const confirmDelete = () =>
    confirmBudgetDelete(budgetName ?? "", name, () => navigate(backHref));

  const gatewayNote =
    "Gateway budget: only name, description and warnings change here.";
  // Each action shows only with its own access: update for Edit and Disable, delete for Delete.
  const actions = budget && (canUpdate || canDelete) && (
    <Stack direction="row" spacing={1}>
      {canUpdate && (
        <Button
          variant="outlined"
          startIcon={<Pencil size={16} />}
          onClick={() => navigate(editHref)}
        >
          Edit
        </Button>
      )}
      {canUpdate && (
        <Tooltip title={fromGateway ? gatewayNote : ""}>
          <span>
            <Button
              variant="outlined"
              startIcon={<Power size={16} />}
              onClick={toggle}
              disabled={fromGateway || isUpdating}
            >
              {inactive ? "Enable" : "Disable"}
            </Button>
          </span>
        </Tooltip>
      )}
      {canDelete && (
        <Tooltip title={fromGateway ? gatewayNote : ""}>
          <span>
            <Button
              variant="outlined"
              color="error"
              startIcon={<Trash2 size={16} />}
              onClick={confirmDelete}
              disabled={fromGateway || isDeleting}
            >
              Delete
            </Button>
          </span>
        </Tooltip>
      )}
    </Stack>
  );

  const unpriced = budget?.status?.currentPeriod?.unpricedRequests ?? 0;
  const lim = budget?.spec.limit;

  return (
    <PageLayout
      title={name}
      description={budget?.spec.description}
      backHref={backHref}
      backLabel="Back to budgets"
      isLoading={isLoading}
      avatar={
        notFound
          ? null
          : { children: <Wallet size={22} />, color: "primary.main" }
      }
      titleTail={
        budget && (
          <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
            <Chip
              size="small"
              variant="outlined"
              color={inactive ? "default" : "success"}
              label={inactive ? "Inactive" : "Active"}
            />
            <Chip
              size="small"
              label={owner ? `Project ${ownerName ?? owner}` : "Organization"}
            />
            {fromGateway && (
              <Chip size="small" variant="outlined" label="From a gateway" />
            )}
          </Stack>
        )
      }
      meta={
        <>
          {budget?.metadata.labels && budget.metadata.labels.length > 0 && (
            <PageMetaItem icon={<Tag size={12} />} label="Labels">
              <LabelChips labels={wordLabels(budget.metadata.labels)} />
            </PageMetaItem>
          )}
          {updatedAt && (
            <PageMetaItem icon={<Clock size={12} />} label="Last updated">
              {`Last updated ${formatRelativeTime(new Date(updatedAt))}`}
            </PageMetaItem>
          )}
        </>
      }
      actions={actions}
    >
      {notFound ? (
        <StateNotice
          icon={SearchX}
          title={`There is no budget named ${budgetName}`}
          description="It may have been deleted, or the link is wrong."
          actions={
            <Button variant="outlined" onClick={() => navigate(backHref)}>
              Back to budgets
            </Button>
          }
        />
      ) : error ? (
        <StateNotice
          icon={ServerCrash}
          tone="error"
          title="Couldn't load the budget"
          description="The server didn't answer. Try again in a moment."
          actions={
            <Button variant="outlined" onClick={() => void refetch()}>
              Retry
            </Button>
          }
        />
      ) : isLoading || !budget || !lim ? (
        <Stack spacing={2.5}>
          <Skeleton variant="rounded" height={260} />
          <Stack direction={{ xs: "column", lg: "row" }} spacing={2.5}>
            <Skeleton variant="rounded" height={340} sx={{ flex: 1 }} />
            <Skeleton variant="rounded" height={340} sx={{ flex: 1 }} />
          </Stack>
        </Stack>
      ) : (
        <Stack spacing={2.5}>
          {fromGateway && <Alert severity="info">{gatewayNote}</Alert>}
          {inactive && (
            <Alert severity="warning">
              Disabled: not counting or blocking until it is enabled.
            </Alert>
          )}
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "1fr",
                lg: "minmax(0, 2fr) minmax(0, 1fr)",
              },
              gap: 2.5,
            }}
          >
            <ThisPeriod budget={budget} />
            <Stack spacing={2.5}>
              <SideCard icon={<Zap size={22} />} title="Enforcement">
                <ActionChip action={lim.action} />
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ mt: 0.75 }}
                >
                  {actionSentence(
                    lim.action,
                    amount(lim.amount),
                    lim.blockAt ?? 100,
                  )}
                </Typography>
              </SideCard>
              <SideCard icon={<Layers size={22} />} title="Unpriced requests">
                <Typography variant="h5" fontWeight={700}>
                  {unpriced}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Charged $0 this period
                </Typography>
                {unpriced > 0 && unpricedHref && canReadCost && (
                  <Link
                    component={RouterLink}
                    to={unpricedLink(unpricedHref, budget, !!readOnly)}
                    underline="hover"
                    variant="body2"
                    sx={{ display: "inline-block", mt: 1 }}
                  >
                    View requests
                  </Link>
                )}
              </SideCard>
            </Stack>
          </Box>
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "1fr",
                lg: "repeat(2, minmax(0, 1fr))",
              },
              gap: 2.5,
            }}
          >
            <Scope budget={budget} orgName={orgName ?? ""} />
            <Alerts budget={budget} orgName={orgName ?? ""} />
          </Box>
          <Allowances budget={budget} orgName={orgName} />
        </Stack>
      )}
    </PageLayout>
  );
}
