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

import { Box, Stack, Typography, useTheme } from "@wso2/oxygen-ui";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ChartTooltip,
  XAxis,
  YAxis,
} from "@wso2/oxygen-ui-charts-react";
import { formatUsd } from "../costFormat";

export interface CostSeries {
  /** Key of the series' values in each row. */
  key: string;
  name: string;
  /** Whether this is the "Other" series (summed smaller groups). */
  other?: boolean;
}

export interface CostChartRow {
  /** Month label shown on the axis. */
  month: string;
  [series: string]: string | number;
}

interface CostChartProps {
  rows: CostChartRow[];
  /** Largest first; "Other", if any, last. */
  series: CostSeries[];
  height?: number;
  /** Widest a month's bar may be, in px (narrow charts beside tiles use less). */
  maxBarSize?: number;
}

/** Gap left between stacked segments, in px. */
const SEGMENT_GAP = 2;
/** Corner radius of the top of each month's bar, in px. */
const TOP_RADIUS = 4;

/** A month's values stacked by size: r0 is its largest, k0 that group's key. */
interface RankedRow {
  month: string;
  /** How many groups have cost this month. */
  count: number;
  [rankOrKey: string]: string | number;
}

interface SegmentProps {
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  payload?: RankedRow;
}

/**
 * The segment at one rank of a month's stack, in the colour of the group that
 * holds that rank, with a transparent gap above it so segments read apart on any
 * surface; the month's top segment gets rounded corners.
 */
function rankShape(rank: number, colorOfKey: (key: string) => string) {
  function Segment(props: unknown) {
    const {
      x = 0,
      y = 0,
      width = 0,
      height = 0,
      payload,
    } = props as SegmentProps;
    const h = height - SEGMENT_GAP;
    if (!payload || h <= 0) return <g />;
    const top = y + SEGMENT_GAP;
    const rounded = rank === payload.count - 1;
    const r = rounded ? Math.min(TOP_RADIUS, h, width / 2) : 0;
    const d = [
      `M${x},${top + h}`,
      `V${top + r}`,
      r ? `Q${x},${top} ${x + r},${top}` : "",
      `H${x + width - r}`,
      r ? `Q${x + width},${top} ${x + width},${top + r}` : "",
      `V${top + h}`,
      "Z",
    ].join(" ");
    return <path d={d} fill={colorOfKey(String(payload[`k${rank}`]))} />;
  }
  return Segment;
}

/** Compact dollar ticks: $0, $500, $3.5k, $14k, $1.2M. */
/**
 * Evenly spaced axis ticks from $0: a round step (1, 2, 2.5 or 5 × a power of ten) giving
 * about four intervals, up to the first tick at or above the highest bar ($4.33 → 0, 2, 4,
 * 6). No raw number becomes a tick; with no cost yet the axis is $0 to $1.
 */
function axisTicks(max: number): number[] {
  if (!(max > 0)) return [0, 0.25, 0.5, 0.75, 1];
  const raw = max / 4;
  const scale = 10 ** Math.floor(Math.log10(raw));
  const m = raw / scale;
  const step =
    (m <= 1 ? 1 : m <= 2 ? 2 : m <= 2.5 ? 2.5 : m <= 5 ? 5 : 10) * scale;
  const count = Math.ceil(max / step - 1e-9);
  return Array.from({ length: count + 1 }, (_, i) => +(i * step).toFixed(6));
}

function axisUsd(value: number): string {
  if (value >= 1_000_000) return `$${+(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `$${+(value / 1_000).toFixed(1)}k`;
  return `$${value}`;
}

function CostTooltip({
  active,
  payload,
  label,
  seriesByKey,
  colorOfKey,
}: {
  active?: boolean;
  payload?: { payload?: RankedRow }[];
  label?: string;
  seriesByKey: Map<string, CostSeries>;
  colorOfKey: (key: string) => string;
}) {
  const row = payload?.[0]?.payload;
  if (!active || !row) return null;
  // The month's groups, largest first, as stacked from the bottom.
  const entries = Array.from({ length: row.count }, (_, i) => ({
    key: String(row[`k${i}`]),
    value: Number(row[`r${i}`]),
  }));
  const total = entries.reduce((sum, e) => sum + e.value, 0);
  return (
    <Box
      sx={{
        // Opaque: the card surface is translucent and would show the bars through.
        bgcolor: "background.default",
        backgroundImage: "none",
        border: 1,
        borderColor: "divider",
        borderRadius: 1.5,
        boxShadow: 4,
        px: 1.5,
        py: 1.25,
        minWidth: 200,
      }}
    >
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Stack spacing={0.5} sx={{ mt: 0.75 }}>
        {entries.map((e) => (
          <Stack key={e.key} direction="row" alignItems="center" spacing={1}>
            <Box
              sx={{
                width: 8,
                height: 8,
                borderRadius: 0.5,
                bgcolor: colorOfKey(e.key),
                flexShrink: 0,
              }}
            />
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ flex: 1 }}
              noWrap
            >
              {seriesByKey.get(e.key)?.name ?? e.key}
            </Typography>
            <Typography
              variant="body2"
              sx={{ fontVariantNumeric: "tabular-nums" }}
            >
              {formatUsd(e.value)}
            </Typography>
          </Stack>
        ))}
      </Stack>
      <Stack
        direction="row"
        justifyContent="space-between"
        sx={{ mt: 1, pt: 1, borderTop: 1, borderColor: "divider" }}
      >
        <Typography variant="body2" color="text.secondary">
          Total
        </Typography>
        <Typography
          variant="body2"
          fontWeight={600}
          sx={{ fontVariantNumeric: "tabular-nums" }}
        >
          {formatUsd(total)}
        </Typography>
      </Stack>
    </Box>
  );
}

/** One stacked bar per month, coloured by group; no animation. */
export function CostChart({
  rows,
  series,
  height = 360,
  maxBarSize = 220,
}: CostChartProps) {
  const theme = useTheme();
  // Series colours from the theme, in a fixed order by the group's rank in the kept
  // months (colour follows the group); "Other" is neutral. Each month is stacked by its
  // own ranking, so the gap between segments, the legend and the tooltip keep them apart.
  const palette = [
    theme.palette.info.main,
    theme.palette.primary.main,
    theme.palette.success.main,
    theme.palette.warning.main,
    theme.palette.secondary.main,
  ];
  const otherColor = theme.palette.text.disabled;
  const ranked = series.filter((s) => !s.other);
  const colorOf = (s: CostSeries) =>
    s.other ? otherColor : palette[ranked.indexOf(s) % palette.length];
  const seriesByKey = new Map(series.map((s) => [s.key, s]));
  const colorOfKey = (key: string) => {
    const s = seriesByKey.get(key);
    return s ? colorOf(s) : otherColor;
  };
  // Each month stacked by its own ranking: its largest group at the bottom.
  const rankedRows: RankedRow[] = rows.map((row) => {
    const entries = series
      .map((s) => ({ key: s.key, value: Number(row[s.key] ?? 0) }))
      .filter((e) => e.value > 0)
      .sort((a, b) => b.value - a.value);
    const stacked: RankedRow = { month: row.month, count: entries.length };
    entries.forEach((e, i) => {
      stacked[`r${i}`] = e.value;
      stacked[`k${i}`] = e.key;
    });
    return stacked;
  });
  const tick = { fill: theme.palette.text.secondary, fontSize: 12 };
  const ticks = axisTicks(
    Math.max(
      0,
      ...rows.map((row) =>
        series.reduce((sum, s) => sum + Number(row[s.key] ?? 0), 0),
      ),
    ),
  );

  return (
    <Stack spacing={2}>
      <Stack
        direction="row"
        flexWrap="wrap"
        useFlexGap
        spacing={2}
        role="list"
        aria-label="Legend"
      >
        {series.map((s) => (
          <Stack
            key={s.key}
            direction="row"
            alignItems="center"
            spacing={0.75}
            role="listitem"
          >
            <Box
              sx={{
                width: 10,
                height: 10,
                borderRadius: 0.75,
                bgcolor: colorOf(s),
              }}
            />
            <Typography variant="body2" color="text.secondary">
              {s.name}
            </Typography>
          </Stack>
        ))}
      </Stack>
      <BarChart
        data={rankedRows}
        xAxisDataKey="month"
        height={height}
        bars={[]}
        grid={{ show: false }}
        xAxis={{ show: false }}
        yAxis={{ show: false }}
        legend={{ show: false }}
        tooltip={{ show: false }}
        isAnimationActive={false}
        margin={{ top: 8, right: 8, left: 0, bottom: 0 }}
        barCategoryGap="16%"
        maxBarSize={maxBarSize}
      >
        <CartesianGrid vertical={false} stroke={theme.palette.divider} />
        <XAxis
          dataKey="month"
          axisLine={false}
          tickLine={false}
          tick={tick}
          tickMargin={10}
        />
        <YAxis
          axisLine={false}
          tickLine={false}
          tick={tick}
          tickFormatter={axisUsd}
          width={56}
          ticks={ticks}
          domain={[0, ticks[ticks.length - 1]]}
        />
        <ChartTooltip
          cursor={{ fill: theme.palette.action.hover }}
          isAnimationActive={false}
          content={
            <CostTooltip seriesByKey={seriesByKey} colorOfKey={colorOfKey} />
          }
        />
        {series.map((_, rank) => (
          <Bar
            key={rank}
            dataKey={`r${rank}`}
            stackId="cost"
            shape={rankShape(rank, colorOfKey)}
            isAnimationActive={false}
          />
        ))}
      </BarChart>
    </Stack>
  );
}
