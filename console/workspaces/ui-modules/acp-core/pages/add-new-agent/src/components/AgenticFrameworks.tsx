/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { Box, Card, CardContent, Chip, Typography } from "@wso2/oxygen-ui";
import { useCallback, useState } from "react";

interface FrameworkOption {
    label: string;
    value: string;
}

const FRAMEWORKS: FrameworkOption[] = [
    { label: "LangChain", value: "langchain" },
    { label: "AutoGen", value: "autogen" },
    { label: "CrewAI", value: "crewai" },
    { label: "LangGraph", value: "langgraph" },
    { label: "OpenAI Swarm", value: "swarm" },
    { label: "LlamaIndex", value: "llamaindex" },
    { label: "Semantic Kernel", value: "semantic-kernel" },
    { label: "Haystack", value: "haystack" },
    { label: "AgentOps", value: "agentops" },
    { label: "Custom Framework", value: "custom" }
];

export const AgenticFrameworks = () => {
    const [selected, setSelected] = useState<Set<string>>(new Set());

    const isSelected = useCallback((value: string) => selected.has(value), [selected]);

    const handleToggle = useCallback((value: string) => {
        setSelected(prev => {
            const next = new Set(prev);
            if (next.has(value)) {
                next.delete(value);
            } else {
                next.add(value);
            }
            return next;
        });
    }, []);

    return (
        <Card variant="outlined">
            <CardContent>
                <Typography variant="h5">
                    Agentic Frameworks
                </Typography>
                <Typography variant="body2" color="text.secondary">
                    Select frameworks used in your agent (optional)
                </Typography>
                <Box display="flex" flexWrap="wrap" gap={1.5} pt={2}>
                    {FRAMEWORKS.map(framework => (
                        <Chip
                            key={framework.value}
                            label={framework.label}
                            color={isSelected(framework.value) ? "primary" : "default"}
                            variant={isSelected(framework.value) ? "filled" : "outlined"}
                            onClick={() => handleToggle(framework.value)}
                            sx={{ cursor: 'pointer' }}
                        />
                    ))}
                </Box>
            </CardContent>
        </Card>
    );
};


