import React from "react";
import { Box, Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";

interface MetricsTooltipPayloadEntry {
    name?: string;
    value?: number;
    color?: string;
    dataKey?: string;
}

interface MetricsTooltipProps {
    active?: boolean;
    payload?: MetricsTooltipPayloadEntry[];
    formatter?: (value: number) => string;
}

const MetricsTooltip: React.FC<MetricsTooltipProps> = ({ active, payload, formatter }) => {
    if (!active || !payload || payload.length === 0) {
        return null;
    }

    return (
        <Card variant="outlined">
            <CardContent>
                <Stack direction="column" gap={0.5}>
                    {payload.map((entry) => (
                        <Stack
                            key={entry.dataKey ?? entry.name}
                            direction="row"
                            alignItems="center"
                            gap={1}
                        >
                            <Box
                                sx={{
                                    width: 8,
                                    height: 8,
                                    borderRadius: "50%",
                                    bgcolor: entry.color ?? "text.secondary",
                                }}
                            />
                            <Typography variant="body2" color="textSecondary" flex={1}>
                                {entry.name ?? entry.dataKey}
                            </Typography>
                            <Typography variant="body2" fontWeight={600}>
                                {typeof entry.value === "number" && formatter
                                    ? formatter(entry.value)
                                    : entry.value ?? "--"}
                            </Typography>
                        </Stack>
                    ))}
                </Stack>
            </CardContent>
        </Card>
    );
};

export default MetricsTooltip;
