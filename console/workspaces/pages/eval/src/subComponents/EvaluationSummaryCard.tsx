import React from "react";
import { Card, CardContent, Chip, Divider, Grid, LinearProgress, Stack, Typography } from "@wso2/oxygen-ui";
import { ArrowUpRight } from "@wso2/oxygen-ui-icons-react";

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
}

const EvaluationSummaryCard: React.FC<EvaluationSummaryCardProps> = ({
    items,
    averageScoreValue,
    averageScoreHelper,
    averageScoreProgress,
}) => (
    <Card variant="outlined">
        <CardContent>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography variant="subtitle1">Evaluation Summary (Last 7 days)</Typography>
                <Chip size="small" label="Auto sampling" />
            </Stack>
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
        </CardContent>
    </Card>
);

export default EvaluationSummaryCard;
