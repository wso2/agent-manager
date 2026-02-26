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
import { Box, Card, CardContent, Typography, Stack } from "@wso2/oxygen-ui";
import { ChartTooltip, RadarChart } from "@wso2/oxygen-ui-charts-react";
import { Activity } from "@wso2/oxygen-ui-icons-react";
import MetricsTooltip from "./MetricsTooltip";

export interface RadarDefinition {
    dataKey: string;
    name: string;
    fillOpacity?: number;
    strokeWidth?: number;
}

interface AgentPerformanceCardProps {
    radarChartData: Array<Record<string, string | number>>;
    radars: RadarDefinition[];
}

const AgentPerformanceCard: React.FC<AgentPerformanceCardProps> =
    ({ radarChartData, radars }) => (
        <Card variant="outlined">
            <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
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
                        <Activity size={48} />
                        <Typography variant="body2" fontWeight={500}>No performance data</Typography>
                        <Typography variant="caption" color="text.secondary" textAlign="center">
                            Run evaluations to see per-evaluator scores here.
                        </Typography>
                    </Box>
                ) : (
                    <>
                        <Box mt={2}>
                            <RadarChart
                                height={396}
                                data={radarChartData}
                                angleKey="metric"
                                radars={radars}
                                legend={{ show: false }}
                                tooltip={{ show: false }}
                            >
                                <ChartTooltip
                                    content={<MetricsTooltip formatter={(value) => `${value.toFixed(1)}%`} />}
                                />
                            </RadarChart>
                        </Box>
                    </>
                )}
            </CardContent>
        </Card>
    );

export default AgentPerformanceCard;
