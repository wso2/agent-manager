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

import React from "react";
import {
  Box,
  Card,
  CardContent,
  Chip,
  Typography,
  Stack,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChartTooltip, RadarChart } from "@wso2/oxygen-ui-charts-react";
import { Activity } from "@wso2/oxygen-ui-icons-react";
import { type EvaluationLevel } from "@agent-management-platform/types";
import { LEVEL_CONFIG, levelChipSx } from "./levelConfig";
import ScoreChip from "./ScoreChip";

export interface RadarDefinition {
  dataKey: string;
  name: string;
  stroke?: string;
  fill?: string;
  fillOpacity?: number;
  strokeWidth?: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  dot?: boolean | React.ReactElement | ((props: any) => React.ReactNode);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  activeDot?: boolean | React.ReactElement | ((props: any) => React.ReactNode);
}

export interface RadarDataPoint {
  metric: string;
  current: number;
  _isNoData: boolean;
  _scoredCount: number;
  _totalCount: number;
  _level: EvaluationLevel;
}

interface AgentPerformanceCardProps {
  radarChartData: RadarDataPoint[];
  radars: RadarDefinition[];
}

const AgentPerformanceCard: React.FC<AgentPerformanceCardProps> = ({
  radarChartData,
  radars,
}) => {
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="subtitle1">Agent Performance</Typography>
        </Stack>
        {radarChartData.length === 0 ? (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={6}
            height={412}
            gap={1}
          >
            <Activity size={36} />
            <Typography variant="body2" fontWeight={500}>
              No performance data
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              Run evaluations to see per-evaluator scores here.
            </Typography>
          </Box>
        ) : (
          <RadarChart
            height={396}
            data={radarChartData}
            angleKey="metric"
            radars={radars}
            legend={{ show: false }}
            tooltip={{ show: false }}
          >
            <ChartTooltip
              cursor={false}
              content={({
                active,
                payload,
              }: {
                active?: boolean;
                payload?: Array<{ value?: number; payload?: RadarDataPoint }>;
              }) => {
                if (!active || !payload?.length) return null;
                const point = payload[0];
                const dataPoint = point.payload;
                if (!dataPoint) return null;

                const cfg = LEVEL_CONFIG[dataPoint._level];

                return (
                  <Card variant="outlined">
                    <CardContent
                      sx={{ py: 1, px: 1.5, "&:last-child": { pb: 1 } }}
                    >
                      <Stack
                        direction="row"
                        alignItems="center"
                        spacing={0.75}
                        mb={0.25}
                      >
                        <Chip
                          label={cfg.label}
                          size="small"
                          sx={levelChipSx(cfg, isDark)}
                        />
                        <Typography variant="body2" fontWeight={600} noWrap>
                          {dataPoint.metric}:{" "}
                          {dataPoint._isNoData ? (
                            "–"
                          ) : typeof point.value === "number" ? (
                            <ScoreChip
                              score={point.value / 100}
                              variant="text"
                            />
                          ) : (
                            "–"
                          )}
                        </Typography>
                      </Stack>
                      {dataPoint._isNoData ? (
                        <Typography variant="caption" color="text.secondary">
                          All {cfg.unit} skipped
                        </Typography>
                      ) : (
                        <Typography variant="caption" color="text.secondary">
                          ({dataPoint._scoredCount}/{dataPoint._totalCount}{" "}
                          {cfg.unit})
                        </Typography>
                      )}
                    </CardContent>
                  </Card>
                );
              }}
            />
          </RadarChart>
        )}
      </CardContent>
    </Card>
  );
};

export default AgentPerformanceCard;
