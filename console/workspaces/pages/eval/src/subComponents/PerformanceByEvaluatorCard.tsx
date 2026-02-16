import React from "react";
import { Box, Button, Card, CardContent, Chip, Divider, Grid, Stack, Typography } from "@wso2/oxygen-ui";
import { ChartTooltip, LineChart } from "@wso2/oxygen-ui-charts-react";
import { Info } from "@wso2/oxygen-ui-icons-react";
import MetricsTooltip from "./MetricsTooltip";

export interface AggregateStats {
    mean: string;
    min: string;
    max: string;
}

export interface EvaluatorTrendPoint {
    label: string;
    score: number;
}

interface PerformanceByEvaluatorCardProps {
    evaluatorTrend: EvaluatorTrendPoint[];
    aggregateStats: AggregateStats;
}

const PerformanceByEvaluatorCard:
    React.FC<PerformanceByEvaluatorCardProps> = ({ evaluatorTrend, aggregateStats }) => (
        <Card variant="outlined">
            <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
                    <Stack spacing={0.5}>
                        <Typography variant="subtitle1">Performance by Evaluator</Typography>
                        <Typography variant="caption" color="text.secondary">
                            Tool Trajectory • Trace • LLM-as-Judge
                        </Typography>
                    </Stack>
                    <Button size="small" variant="outlined" startIcon={<Info size={16} />}>
                        View All Traces
                    </Button>
                </Stack>
                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, md: 8 }}>
                        <LineChart
                            height={280}
                            data={evaluatorTrend}
                            xAxisDataKey="label"
                            lines={[
                                {
                                    dataKey: "score",
                                    name: "Current Score",
                                    stroke: "#3f8cff",
                                    strokeWidth: 2,
                                    dot: false,
                                },
                            ]}
                            grid={{ show: true, strokeDasharray: "3 3" }}
                            tooltip={{ show: false }}
                        >
                            <ChartTooltip
                                content={
                                    <MetricsTooltip formatter={(value) => value.toFixed(2)} />
                                }
                            />
                        </LineChart>
                    </Grid>
                    <Grid size={{ xs: 12, md: 4 }}>
                        <Stack spacing={1.5}>
                            <Typography variant="caption" color="text.secondary">
                                Aggregated stats (7 days)
                            </Typography>
                            <Stack direction="row" spacing={2}>
                                <Box>
                                    <Typography variant="h4">{aggregateStats.mean}</Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        Mean
                                    </Typography>
                                </Box>
                                <Divider orientation="vertical" flexItem />
                                <Box>
                                    <Typography variant="body2">Min: {aggregateStats.min}</Typography>
                                    <Typography variant="body2">Max: {aggregateStats.max}</Typography>
                                </Box>
                            </Stack>
                            <Chip color="success" size="small" label="Current Score 0.94" />
                            <Typography variant="body2" color="text.secondary">
                                Evaluators flagged 2
                                regressions. Latency is the largest contributor to the drop.
                            </Typography>
                        </Stack>
                    </Grid>
                </Grid>
            </CardContent>
        </Card>
    );

export default PerformanceByEvaluatorCard;
