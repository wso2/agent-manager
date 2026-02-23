import React from "react";
import { Box, Card, CardContent, Chip, Divider, Grid, LinearProgress, Stack, Typography, Button } from "@wso2/oxygen-ui";
import { Activity, ArrowUpRight, History } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams, Link as RouterLink } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

export interface EvaluationSummaryItem {
    label: string;
    value: string;
    helper: string;
    trend: number;
}

interface EvaluationSummaryCardProps {
    items: EvaluationSummaryItem[];
    averageScoreValue: string;
    averageScoreHelper: string;
    averageScoreProgress: number;
    timeRangeLabel?: string;
}

const EvaluationSummaryCard: React.FC<EvaluationSummaryCardProps> = ({
    items,
    averageScoreValue,
    averageScoreHelper,
    averageScoreProgress,
    timeRangeLabel,
}) => {
    const { orgId, projectId, agentId, monitorId } = useParams<{
        orgId: string;
        projectId: string;
        agentId: string;
        monitorId: string;
    }>();

    return (
        <Card variant="outlined">
            <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Typography variant="subtitle1">Evaluation Summary ({timeRangeLabel ?? "Last 7 days"})</Typography>
                    <Button
                        variant="text"
                        component={RouterLink}
                        startIcon={<History size={16} />}
                        to={
                            generatePath(
                                absoluteRouteMap.children.org.children
                                    .projects.children.agents.children
                                    .evaluation.children.monitor.children.view.children.runs.path,
                                { orgId, projectId, agentId, monitorId })
                        }
                    >
                        Run History
                    </Button>
                </Stack>
                {items.length === 0 ? (
                    <Box
                        display="flex"
                        flexDirection="column"
                        alignItems="center"
                        justifyContent="center"
                        py={4}
                        gap={1}
                    >
                        <Activity size={48} />
                        <Typography variant="body2" fontWeight={500}>No evaluation data</Typography>
                        <Typography variant="caption" color="text.secondary" textAlign="center">
                            Scores will appear here once evaluations complete.
                        </Typography>
                    </Box>
                ) : (
                    <>
                        <Grid container spacing={2} mt={1}>
                            {items.map((item) => (
                                <Grid key={item.label} size={{ xs: 12, sm: 4 }}>
                                    <Stack spacing={0.5}>
                                        <Typography variant="caption" color="text.secondary">
                                            {item.label}
                                        </Typography>
                                        <Stack direction="row" alignItems="center" spacing={0.75}>
                                            <Typography variant="h5">{item.value}</Typography>
                                            <Chip
                                                size="small"
                                                color={item.trend >= 0 ? "success" : "error"}
                                                variant="outlined"
                                                icon={<ArrowUpRight size={12} />}
                                                label={`${item.trend >= 0 ? "↑" : "↓"}${Math.abs(item.trend)}%`}
                                            />
                                        </Stack>
                                        <Typography variant="caption" color="text.secondary">
                                            {item.helper}
                                        </Typography>
                                    </Stack>
                                </Grid>
                            ))}
                        </Grid>
                        <Divider sx={{ my: 2 }} />
                        <Stack spacing={1}>
                            <Typography variant="caption" color="text.secondary">
                                Average Score
                            </Typography>
                            <Stack direction="row" alignItems="center" spacing={1}>
                                <Typography variant="h3">{averageScoreValue}</Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {averageScoreHelper}
                                </Typography>
                            </Stack>
                            <LinearProgress variant="determinate" value={averageScoreProgress} />
                        </Stack>
                    </>
                )}
            </CardContent>
        </Card>
    )
};

export default EvaluationSummaryCard;
