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

import { useMemo } from "react";
import {
  Button,
  Card,
  CardContent,
  Grid,
  Skeleton,
  Link,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChartColumnStacked,
  ExternalLink,
  ServerCrash,
} from "@wso2/oxygen-ui-icons-react";
import { Link as RouterLink } from "react-router-dom";
import { StateNotice } from "@agent-management-platform/views";
import { globalConfig } from "@agent-management-platform/types";
import {
  amount,
  changePercent,
  entryMonthly,
  formatUsdCents,
  groupKey,
  groupLabel,
  keptMonths,
  monthLabel,
  monthlyTotals,
  unpricedTotals,
  zeroCostNote,
} from "../costFormat";
import { CostFilterBar, useCostSelection } from "./CostFilters";
import { CostChart, type CostChartRow, type CostSeries } from "./CostChart";

/** Groups shown on their own in the chart; the rest are summed as "Other". */
const TOP_GROUPS = 5;

interface CostOverviewProps {
  orgName?: string;
  /** Set on a project's page: only its cost, grouped by what is below a project. */
  projectName?: string;
  /** Set on an agent's page (with its project): only that agent's cost. */
  agentName?: string;
  /** The Requests charged $0 page; the Dashboard selection goes with it. */
  unpricedHref?: string;
}

function Tile({
  label,
  value,
  caption,
  highlight,
  action,
}: {
  label: string;
  value: string;
  caption?: string;
  highlight?: boolean;
  action?: React.ReactNode;
}) {
  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        borderColor: highlight ? "primary.main" : "divider",
      }}
    >
      <CardContent>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography
          variant="h5"
          fontWeight={700}
          sx={{
            mt: 0.5,
            fontSize: "1.75rem",
            color: highlight ? "primary.main" : "text.primary",
            fontVariantNumeric: "tabular-nums",
          }}
        >
          {value}
        </Typography>
        {(caption || action) && (
          <Typography variant="caption" color="text.secondary">
            {caption}
            {caption && action ? " · " : ""}
            {action}
          </Typography>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * "Advanced analytics", opening the deployment's analytics dashboard (Moesif or any
 * other tool) in a new tab; hidden when the deployment names none.
 */
function AnalyticsLink() {
  const url = globalConfig?.analyticsDashboardUrl?.trim();
  if (!url) {
    return null;
  }
  // Leaves the AI Control Plane, as the console's other external links do.
  return (
    <Button
      variant="outlined"
      size="small"
      component="a"
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      endIcon={<ExternalLink size={14} />}
    >
      Advanced analytics
    </Button>
  );
}

/** What LLM traffic cost in the kept months, grouped and picked by one dimension. */
export function CostOverview({
  orgName,
  projectName,
  agentName,
  unpricedHref,
}: CostOverviewProps) {
  const selection = useCostSelection(orgName, projectName, agentName);
  const {
    projectScoped,
    groupBy,
    picked,
    names,
    allGroups,
    isLoading,
    error,
    searchParams,
  } = selection;
  const months = useMemo(() => keptMonths(allGroups), [allGroups]);
  const groups = useMemo(
    () =>
      picked.length
        ? allGroups.filter((g) => picked.includes(groupKey(g, groupBy)))
        : allGroups,
    [allGroups, picked, groupBy],
  );

  const totals = monthlyTotals(groups, months);
  const current = totals[months.length - 1] ?? 0;
  const last = totals[months.length - 2] ?? 0;
  const beforeLast = totals[months.length - 3] ?? 0;
  const threeMonths = totals.reduce((a, b) => a + b, 0);
  const change = changePercent(beforeLast, last);
  const { missingPrice: missing, usageUnavailable: unavailable } =
    unpricedTotals(groups);

  // The top groups by total, then everything else as one "Other" series.
  // Groups that cost nothing (no price, no usage) have no bar; they are named under
  // the chart instead of crowding the legend.
  const ranked = groups
    .filter((g) => amount(g.cost) > 0)
    .sort((a, b) => amount(b.cost) - amount(a.cost));
  const zeroCost = groups
    .filter((g) => amount(g.cost) <= 0)
    .map((g) =>
      groupLabel(groupKey(g, groupBy), groupBy, projectScoped, names),
    );
  const top = ranked.slice(0, TOP_GROUPS);
  const rest = ranked.slice(TOP_GROUPS);
  const series: (CostSeries & { costs: number[] })[] = top.map((g, i) => ({
    key: `s${i}`,
    name: groupLabel(groupKey(g, groupBy), groupBy, projectScoped, names),
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
  const chartRows: CostChartRow[] = months.map((m, i) => {
    const row: CostChartRow = {
      month:
        i === months.length - 1 ? `${monthLabel(m)} (so far)` : monthLabel(m),
    };
    series.forEach((s) => {
      // Exact amounts: the tooltip adds them, and rounds only what it shows.
      row[s.key] = s.costs[i];
    });
    return row;
  });

  const thisMonth = months.length ? monthLabel(months[months.length - 1]) : "";
  const lastMonth =
    months.length > 1 ? monthLabel(months[months.length - 2]) : "";
  const firstMonth = months.length ? monthLabel(months[0]) : "";

  if (error) {
    return (
      <StateNotice
        compact
        icon={ServerCrash}
        tone="error"
        title="Couldn't load cost"
        description="Couldn't load costs. Reload to try again."
      />
    );
  }

  return (
    <Stack spacing={2.5}>
      <CostFilterBar selection={selection} />

      <Grid container spacing={2}>
        {isLoading ? (
          [0, 1, 2, 3].map((i) => (
            <Grid key={i} size={{ xs: 12, sm: 6, lg: 3 }}>
              <Skeleton variant="rounded" height={104} />
            </Grid>
          ))
        ) : (
          <>
            <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
              <Tile
                label={`This month (${thisMonth})`}
                value={formatUsdCents(current)}
                caption={`So far in ${thisMonth}`}
                highlight
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
              <Tile
                label={`Last month (${lastMonth})`}
                value={formatUsdCents(last)}
                caption={
                  change === undefined
                    ? undefined
                    : `${change >= 0 ? "+" : ""}${change}% vs ${monthLabel(months[months.length - 3] ?? "")}`
                }
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
              <Tile
                label={`Last 3 months (${firstMonth}–${thisMonth})`}
                value={formatUsdCents(threeMonths)}
                caption={`Includes ${thisMonth} so far`}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
              <Tile
                label={`Requests charged $0 (${firstMonth}–${thisMonth})`}
                value={String(missing + unavailable)}
                caption={`${missing} no price · ${unavailable} no usage`}
                action={
                  unpricedHref && (
                    <Link
                      component={RouterLink}
                      to={`${unpricedHref}?${searchParams.toString()}`}
                      underline="hover"
                    >
                      Details
                    </Link>
                  )
                }
              />
            </Grid>
          </>
        )}
      </Grid>

      <Card variant="outlined">
        <CardContent>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
            sx={{ mb: 1 }}
          >
            <Typography variant="subtitle1">Monthly cost (USD)</Typography>
            <AnalyticsLink />
          </Stack>
          {isLoading ? (
            <Skeleton variant="rounded" height={396} />
          ) : groups.length === 0 ? (
            <StateNotice
              compact
              icon={ChartColumnStacked}
              title="No AI traffic yet"
              description="Costs appear once requests flow through a gateway."
            />
          ) : (
            <CostChart rows={chartRows} series={series} />
          )}
          {zeroCost.length > 0 && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", mt: 1 }}
            >
              {zeroCostNote(zeroCost)}
            </Typography>
          )}
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: "block", mt: zeroCost.length ? 0.25 : 1 }}
          >
            UTC months · last 3 months
          </Typography>
        </CardContent>
      </Card>
    </Stack>
  );
}
