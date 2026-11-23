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
  Box,
  Card,
  CardContent,
  Chip,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { useCosts } from "@agent-management-platform/api-client";
import { NESTED_CARD_SX } from "@agent-management-platform/shared-component";
import {
  amount,
  changePercent,
  type CostNames,
  zeroCostNote,
  entryMonthly,
  formatUsdCents,
  groupKey,
  groupLabel,
  keptMonths,
  monthLabel,
  monthlyTotals,
} from "../AICostManagement/costFormat";
import {
  CostChart,
  type CostChartRow,
  type CostSeries,
} from "../AICostManagement/subComponents/CostChart";
import type { CostGroupBy } from "@agent-management-platform/types";

export interface EntityCostPanelProps {
  orgName: string;
  /** The entity's cost filter, e.g. `project:payments` or `agent:payments/checkout`. */
  filter: string;
}

/**
 * A metric tile: its label, the amount beneath it as the headline, and an optional
 * adornment on the right. A highlighted tile (this month) shows its amount in the
 * accent colour, with an accent edge.
 */
function MetricTile({
  value,
  label,
  adornment,
  loading,
  highlight,
}: {
  value: string;
  label: string;
  adornment?: ReactNode;
  loading?: boolean;
  highlight?: boolean;
}) {
  return (
    <Card
      variant="outlined"
      sx={{
        ...NESTED_CARD_SX,
        width: "100%",
        height: "100%",
        ...(highlight ? { borderColor: "primary.main" } : {}),
      }}
    >
      <CardContent
        sx={{
          py: 1.5,
          px: 2,
          "&:last-child": { pb: 1.5 },
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1,
          minHeight: 64,
          height: "100%",
        }}
      >
        <Box sx={{ minWidth: 0 }}>
          <Typography
            variant="body2"
            color="text.secondary"
            noWrap
            component="div"
          >
            {label}
          </Typography>
          {loading ? (
            <Skeleton variant="text" width={96} height={36} />
          ) : (
            <Typography
              variant="h5"
              fontWeight={700}
              lineHeight={1.25}
              color={highlight ? "primary.main" : "text.primary"}
              sx={{ fontSize: "1.75rem", fontVariantNumeric: "tabular-nums" }}
            >
              {value}
            </Typography>
          )}
        </Box>
        {!loading && adornment}
      </CardContent>
    </Card>
  );
}

/** Up for more spend, down for less; neutral when unchanged. */
function ChangeChip({ change, against }: { change: number; against: string }) {
  const color = change > 0 ? "warning" : change < 0 ? "success" : "default";
  return (
    <Chip
      size="small"
      variant="outlined"
      color={color}
      label={`${change > 0 ? "+" : ""}${change}% vs ${against}`}
      sx={{ flexShrink: 0 }}
    />
  );
}

/** One entity's monthly cost figures: this month so far, last month and its
 * change from the month before, and the three kept months (UTC calendar months). */
export function useEntityCost(orgName: string, filter: string) {
  const { data, isLoading, error } = useCosts(
    { orgName },
    { groupBy: "organization", filter: [filter] },
  );
  const entries = data?.costByGroup ?? [];
  const months = keptMonths(entries);
  const totals = monthlyTotals(entries, months);
  const n = months.length;
  const label = (i: number) => (months[i] ? monthLabel(months[i]) : "");
  const change = changePercent(totals[n - 3] ?? 0, totals[n - 2] ?? 0);
  return {
    isLoading,
    error,
    current: totals[n - 1] ?? 0,
    last: totals[n - 2] ?? 0,
    window: totals.reduce((a, b) => a + b, 0),
    thisMonth: label(n - 1),
    lastMonth: label(n - 2),
    firstMonth: label(0),
    beforeLast: label(n - 3),
    change,
    totals,
    /** e.g. "+13% vs Sep"; undefined when the month before had no cost. */
    changeText:
      change === undefined
        ? undefined
        : `${change >= 0 ? "+" : ""}${change}% vs ${label(n - 3)}`,
  };
}

/** The three figures as tiles, side by side (agent and LLM provider pages). */
export function EntityCostTiles({
  orgName,
  filter,
  stacked,
}: EntityCostPanelProps & {
  /** One under another, beside a chart. */
  stacked?: boolean;
}) {
  const c = useEntityCost(orgName, filter);
  if (c.error) {
    return (
      <Typography variant="body2" color="text.secondary">
        Couldn&apos;t load cost. Reload the page to try again.
      </Typography>
    );
  }
  const tiles = [
    <MetricTile
      key="current"
      value={formatUsdCents(c.current)}
      label={`This month (${c.thisMonth}) so far`}
      loading={c.isLoading}
      highlight
    />,
    <MetricTile
      key="last"
      value={formatUsdCents(c.last)}
      label={`Last month (${c.lastMonth})`}
      adornment={
        c.change !== undefined && (
          <ChangeChip change={c.change} against={c.beforeLast} />
        )
      }
      loading={c.isLoading}
    />,
    <MetricTile
      key="window"
      value={formatUsdCents(c.window)}
      label={`Last 3 months (${c.firstMonth}–${c.thisMonth})`}
      loading={c.isLoading}
    />,
  ];
  if (stacked) {
    // One under another, sharing the column's height equally, so they line up with the
    // chart beside them.
    return (
      <Stack spacing={1.5} sx={{ height: "100%" }}>
        {tiles.map((tile) => (
          <Box key={tile.key} sx={{ flex: 1, minHeight: 0 }}>
            {tile}
          </Box>
        ))}
      </Stack>
    );
  }
  return (
    <Grid container spacing={1.5}>
      {tiles.map((tile) => (
        <Grid key={tile.key} size={{ xs: 12, sm: 4 }}>
          {tile}
        </Grid>
      ))}
    </Grid>
  );
}

/** Groups shown on their own in an entity's chart; the rest are summed as "Other". */
const CHART_GROUPS = 5;

/**
 * One entity's monthly cost as the Dashboard's chart, stacked by `groupBy` (an agent's
 * cost by model): the three kept UTC months, the current one so far.
 */
export function EntityCostChart({
  orgName,
  filter,
  groupBy,
  height = 260,
  names,
  bare,
  fill,
  maxBarSize,
}: EntityCostPanelProps & {
  groupBy: CostGroupBy;
  height?: number;
  /** Display names for the groups (agents, projects, providers). */
  names?: CostNames;
  /** Inside another card (a side column): no card of its own. */
  bare?: boolean;
  /** The card fills its parent's height (beside the stacked tiles). */
  fill?: boolean;
  /** Widest a month's bar may be, in px. */
  maxBarSize?: number;
}) {
  const { data, isLoading, error } = useCosts(
    { orgName },
    { groupBy, filter: [filter] },
  );
  if (error) return null; // the tiles above already say cost couldn't be loaded
  if (isLoading) return <Skeleton variant="rounded" height={height} />;
  const entries = data?.costByGroup ?? [];
  const months = keptMonths(entries);
  // Groups that cost nothing (no price, no usage) would only crowd the legend.
  const ranked = entries
    .filter((g) => amount(g.cost) > 0)
    .sort((a, b) => amount(b.cost) - amount(a.cost));
  const zeroCost = entries
    .filter((g) => amount(g.cost) <= 0)
    .map((g) => groupLabel(groupKey(g, groupBy), groupBy, true, names));
  const top = ranked.slice(0, CHART_GROUPS);
  const rest = ranked.slice(CHART_GROUPS);
  const series: (CostSeries & { costs: number[] })[] = top.map((g, i) => ({
    key: `s${i}`,
    name: groupLabel(groupKey(g, groupBy), groupBy, true, names),
    costs: entryMonthly(g, months),
  }));
  if (rest.length) {
    series.push({
      key: "other",
      name: `Other (${rest.length})`,
      other: true,
      costs: monthlyTotals(rest, months),
    });
  }
  const rows: CostChartRow[] = months.map((m, i) => {
    const row: CostChartRow = {
      month:
        i === months.length - 1 ? `${monthLabel(m)} (so far)` : monthLabel(m),
    };
    // Exact amounts: the tooltip adds them, and rounds only what it shows.
    series.forEach((s) => {
      row[s.key] = s.costs[i];
    });
    return row;
  });
  const title = (
    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
      Monthly cost by {groupBy === "llmProvider" ? "LLM provider" : groupBy}{" "}
      (USD)
    </Typography>
  );
  // With no cost the chart is still drawn, empty, over the three months.
  const chart = (
    <>
      <CostChart
        rows={rows}
        series={series}
        height={height}
        maxBarSize={maxBarSize}
      />
      {zeroCost.length > 0 && (
        <Typography variant="caption" color="text.secondary">
          {zeroCostNote(zeroCost)}
        </Typography>
      )}
    </>
  );
  if (bare) {
    return (
      <Box>
        {title}
        {chart}
      </Box>
    );
  }
  return (
    <Card
      variant="outlined"
      sx={{ ...NESTED_CARD_SX, ...(fill ? { height: "100%" } : {}) }}
    >
      <CardContent>
        {title}
        {chart}
      </CardContent>
    </Card>
  );
}
