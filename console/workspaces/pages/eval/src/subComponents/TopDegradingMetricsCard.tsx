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
import { Box, Card, CardContent, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import { Activity } from "@wso2/oxygen-ui-icons-react";

export interface DegradingMetric {
    label: string;
    delta: number;
    range: string;
}

interface TopDegradingMetricsCardProps {
    metrics: DegradingMetric[];
}

const TopDegradingMetricsCard: React.FC<TopDegradingMetricsCardProps> = ({ metrics }) => (
    <Card variant="outlined">
        <CardContent>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography variant="subtitle2">Top degrading metrics</Typography>
                {metrics.length > 0 && <Chip size="small" color="warning" variant="outlined" label="Investigate" />}
            </Stack>
            {metrics.length === 0 ? (
                <Box
                    display="flex"
                    flexDirection="column"
                    alignItems="center"
                    justifyContent="center"
                    py={4}
                    gap={1}
                >
                    <Activity size={48} />
                    <Typography variant="body2" fontWeight={500}>No degrading metrics</Typography>
                    <Typography variant="caption" color="text.secondary" textAlign="center">
                        All evaluators are stable vs the 30-day baseline.
                    </Typography>
                </Box>
            ) : (
                <Stack spacing={1.5} mt={1.5}>
                    {metrics.map((metric) => (
                        <Stack key={metric.label} direction="row" justifyContent="space-between" alignItems="center">
                            <Box>
                                <Typography variant="body2">{metric.label}</Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {metric.range}
                                </Typography>
                            </Box>
                            <Chip color="error" size="small" variant="outlined" label={`${metric.delta}%`} />
                        </Stack>
                    ))}
                </Stack>
            )}
        </CardContent>
    </Card>
);

export default TopDegradingMetricsCard;
