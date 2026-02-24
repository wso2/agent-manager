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
