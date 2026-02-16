import React from "react";
import { Box, Card, CardContent, Chip, Stack, Typography } from "@wso2/oxygen-ui";

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
                <Chip size="small" color="warning" label="Investigate" />
            </Stack>
            <Stack spacing={1.5} mt={1.5}>
                {metrics.map((metric) => (
                    <Stack key={metric.label} direction="row" justifyContent="space-between" alignItems="center">
                        <Box>
                            <Typography variant="body2">{metric.label}</Typography>
                            <Typography variant="caption" color="text.secondary">
                                {metric.range}
                            </Typography>
                        </Box>
                        <Chip color="error" size="small" label={`${metric.delta}%`} />
                    </Stack>
                ))}
            </Stack>
        </CardContent>
    </Card>
);

export default TopDegradingMetricsCard;
