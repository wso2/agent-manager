import React from "react";
import { Box, Card, CardContent, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import { ChartTooltip, RadarChart } from "@wso2/oxygen-ui-charts-react";
import MetricsTooltip from "./MetricsTooltip";

export interface ScoreBadge {
    label: string;
    score: string;
    intent: "default" | "primary" | "success" | "info" | "warning" | "error";
}

export interface RadarDefinition {
    dataKey: string;
    name: string;
    fillOpacity?: number;
    strokeWidth?: number;
}

interface AgentPerformanceCardProps {
    radarChartData: Array<Record<string, string | number>>;
    radars: RadarDefinition[];
    scoreBadges: ScoreBadge[];
}

const AgentPerformanceCard: React.FC<AgentPerformanceCardProps> =
    ({ radarChartData, radars, scoreBadges }) => (
        <Card variant="outlined">
            <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Typography variant="subtitle1">Agent Performance</Typography>
                    <Chip label="Compare: v2.3.8" size="small" />
                </Stack>
                <Box mt={2}>
                    <RadarChart
                        height={385}
                        data={radarChartData}
                        angleKey="metric"
                        radars={radars}
                        legend={{ show: true, align: "center" }}
                        tooltip={{ show: false }}
                    >
                        <ChartTooltip
                            content={<MetricsTooltip formatter={(value) => `${Math.round(value * 100)}%`} />}
                        />
                    </RadarChart>
                </Box>
                <Stack direction="row" spacing={1} flexWrap="wrap" mt={2}>
                    {scoreBadges.map((badge) => (
                        <Chip
                            key={badge.label}
                            color={badge.intent}
                            variant="outlined"
                            label={`${badge.label}: ${badge.score}`}
                            size="small"
                        />
                    ))}
                </Stack>
            </CardContent>
        </Card>
    );

export default AgentPerformanceCard;
