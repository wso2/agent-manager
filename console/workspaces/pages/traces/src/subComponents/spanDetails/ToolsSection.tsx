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

import { NoDataFound } from "@agent-management-platform/views";
import {
  Box,
  Card,
  CardContent,
  Stack,
  Typography,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@wso2/oxygen-ui";
import { Shovel } from "@wso2/oxygen-ui-icons-react";
import { ToolDefinition } from "@agent-management-platform/types";
import { useCallback, useState } from "react";

interface ToolsSectionProps {
  tools: ToolDefinition[];
}

export function ToolsSection({ tools }: ToolsSectionProps) {
  const [expandedToolId, setExpandedToolId] = useState<string | false>(false);

  const handleAccordionChange = useCallback(
    (toolId: string) => (_event: React.SyntheticEvent, isExpanded: boolean) => {
      setExpandedToolId(isExpanded ? toolId : false);
    },
    []
  );

  const isJsonObject = useCallback((value: unknown): boolean => {
    if (value === null || value === undefined) return false;

    if (typeof value === "object" || Array.isArray(value)) return true;

    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        return typeof parsed === "object" && parsed !== null;
      } catch {
        return false;
      }
    }

    return false;
  }, []);

  const renderParameterValue = useCallback((value: unknown): string => {
    if (value === null) return "null";
    if (value === undefined) return "undefined";

    if (typeof value === "object" || Array.isArray(value)) {
      try {
        return JSON.stringify(value, null, 2);
      } catch {
        return String(value);
      }
    }

    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        if (typeof parsed === "object" && parsed !== null) {
          return JSON.stringify(parsed, null, 2);
        }
      } catch {
        // Not valid JSON, return as-is
      }
    }

    return String(value);
  }, []);

  if (!tools || tools.length === 0) {
    return (
      <NoDataFound
        message="No tools found"
        iconElement={Shovel}
        subtitle="No tools found"
        disableBackground
      />
    );
  }

  return (
    <Stack>
      {tools.map((tool, index) => {
        const toolId = tool.name || `tool-${index}`;
        const isExpanded = expandedToolId === toolId;

        return (
          <Accordion
            key={toolId}
            expanded={isExpanded}
            onChange={handleAccordionChange(toolId)}
          >
            <AccordionSummary>
              <Typography variant="h6">{tool.name}</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack direction="column" spacing={2}>
                {tool.description && (
                  <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Description
                    </Typography>
                    <Card
                      variant="outlined"
                    >
                      <CardContent sx={{ "&:last-child": { pb: 1.5 } }}>
                        <Typography variant="body2">
                          {tool.description}
                        </Typography>
                      </CardContent>
                    </Card>
                  </Box>
                )}
                {tool.parameters && (
                  <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Parameters
                    </Typography>
                    <Card
                      variant="outlined"
                      sx={{
                        maxHeight: isJsonObject(tool.parameters)
                          ? 375
                          : "auto",
                        overflow: "auto",
                
                      }}
                    >
                      <CardContent sx={{ "&:last-child": { pb: 1.5 } }}>
                        <Typography
                          variant="caption"
                          sx={{
                            fontFamily: "monospace",
                            whiteSpace: "pre-wrap",
                            wordBreak: "break-word",
                          }}
                        >
                          {renderParameterValue(tool.parameters)}
                        </Typography>
                      </CardContent>
                    </Card>
                  </Box>
                )}
              </Stack>
            </AccordionDetails>
          </Accordion>
        );
      })}
    </Stack>
  );
}
