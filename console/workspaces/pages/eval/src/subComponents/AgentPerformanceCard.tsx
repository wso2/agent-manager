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
                                height={385}
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
