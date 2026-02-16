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

import { AdapterDateFns, Box, Button, CardContent, CardHeader, Chip, Collapse, DatePickers, Form, SearchBar, Stack, TextField, Tooltip, Typography } from "@wso2/oxygen-ui";
import { GitHub, History, Timer } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";


const mockMonitorData = [
    {
        id: "1",
        name: "High Latency Monitor",
        description: "Alerts when latency exceeds 500ms",
        tags: ["latency", "performance"],
    },
    {
        id: "2",
        name: "Error Rate Monitor",
        description: "Alerts when error rate exceeds 5%",
        tags: ["errors", "stability"],
    },
    {
        id: "3",
        name: "Token Usage Spike",
        description: "Detects sudden increases in token consumption",
        tags: ["tokens", "cost"],
    },
    {
        id: "4",
        name: "Slow Build Monitor",
        description: "Flags builds that exceed configured durations",
        tags: ["build", "ci"],
    },
    {
        id: "5",
        name: "Gateway Timeout Monitor",
        description: "Tracks 504 responses returned by the gateway",
        tags: ["gateway", "timeouts"],
    },
    {
        id: "6",
        name: "Memory Pressure Monitor",
        description: "Warns when pod memory utilization crosses 80%",
        tags: ["memory", "k8s"],
    },
    {
        id: "7",
        name: "CPU Saturation Monitor",
        description: "Alerts when CPU usage remains above 90%",
        tags: ["cpu", "k8s"],
    },
    {
        id: "8",
        name: "Deployment Failure Monitor",
        description: "Notifies when deployments fail consecutively",
        tags: ["deploy", "reliability"],
    },
    {
        id: "9",
        name: "Trace Latency Monitor",
        description: "Checks p95 span latency for regressions",
        tags: ["traces", "latency"],
    },
    {
        id: "10",
        name: "Queue Backlog Monitor",
        description: "Tracks message queue backlogs over the limit",
        tags: ["queue", "throughput"],
    },
    {
        id: "11",
        name: "Secrets Rotation Monitor",
        description: "Warns when secrets are nearing expiration",
        tags: ["security", "compliance"],
    },
    {
        id: "12",
        name: "Anomaly Detection Monitor",
        description: "Uses z-score to flag anomalous response times",
        tags: ["ml", "anomaly"],
    },
    {
        id: "13",
        name: "Throughput Regression Monitor",
        description: "Detects drops in processed requests per minute",
        tags: ["throughput", "performance"],
    },
    {
        id: "14",
        name: "Auth Failure Monitor",
        description: "Notifies when authentication failures spike",
        tags: ["auth", "security"],
    },
    {
        id: "15",
        name: "Model Drift Monitor",
        description: "Alerts when evaluation metrics fall below SLA",
        tags: ["model", "quality"],
    },
];
export function CreateMonitorForm() {
    // TempState
    const [ds, setDs] = useState("previous");
    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header>
                    Basic Details
                </Form.Header>
                <Form.ElementWrapper name="name" label="Name" >
                    <TextField name="name" placeholder="Enter monitor name" required fullWidth />
                </Form.ElementWrapper>
                <Form.ElementWrapper name="description" label="Description" >
                    <TextField name="description" placeholder="Enter monitor description" fullWidth multiline minRows={3} />
                </Form.ElementWrapper>
            </Form.Section>
            <Form.Section>
                <Form.Header>
                    Data Collection
                </Form.Header>
                <Form.Stack direction="row">
                    <Form.CardButton onClick={() => setDs("previous")} selected={ds === "previous"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <History size={24} />
                            <Form.Body>Past Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                    <Form.CardButton onClick={() => setDs("future")} selected={ds === "future"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <Timer size={24} />
                            <Form.Body>Future Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                </Form.Stack>
                <Collapse in={ds === "previous"}>
                    <Form.Stack direction="row" >
                        <Form.ElementWrapper name="startTime" label="Start Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                        <Form.ElementWrapper name="endTime" label="End Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                    </Form.Stack>
                </Collapse>
            </Form.Section>
        </Form.Stack>

    )
}

export function SelectPresetMonitors() {
    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header   >
                    <Stack direction="row" spacing={1} alignItems="start" justifyContent="space-between">
                        Preset Monitors
                        <SearchBar />
                    </Stack>
                </Form.Header>
                <Box
                    sx={{
                        display: "grid",
                        gridTemplateColumns: {
                            xs: "repeat(auto-fill, minmax(260px, 1fr))",
                            md: "repeat(auto-fill, minmax(300px, 1fr))",
                        },
                        gap: 2,
                    }}
                >
                    {
                        mockMonitorData.map(monitor => (
                            <Form.CardButton key={monitor.id} sx={{ width: "100%" }}>
                                <CardHeader title={<Stack direction="row" spacing={1} alignItems="center">
                                    <Stack direction="column" spacing={2}>
                                        <Stack direction="row" spacing={2} alignItems="center">
                                            <Typography variant="h5" textOverflow="ellipsis" overflow="hidden" whiteSpace="nowrap" maxWidth="90%">
                                                {monitor.name}
                                            </Typography>
                                            <Form.DisappearingCardButtonContent>
                                                <Tooltip title="What is this?">
                                                    <GitHub size={16} />
                                                </Tooltip>
                                            </Form.DisappearingCardButtonContent>
                                        </Stack>
                                        <Stack direction="row" spacing={1} alignItems="center">
                                            {monitor.tags.map(tag => (
                                                <Chip key={tag} size="small" label={tag} variant="outlined" />
                                            ))}
                                        </Stack>
                                    </Stack>
                                </Stack>} />
                                <CardContent>
                                    <Stack spacing={1}>
                                        <Typography variant="caption">
                                            {monitor.description}
                                        </Typography>
                                        <Form.DisappearingCardButtonContent>
                                            <Button variant="outlined" size="small" color="primary">
                                                Select
                                            </Button>
                                        </Form.DisappearingCardButtonContent>
                                    </Stack>
                                </CardContent>

                            </Form.CardButton>
                        ))
                    }
                </Box>
            </Form.Section>
        </Form.Stack>
    )
}

