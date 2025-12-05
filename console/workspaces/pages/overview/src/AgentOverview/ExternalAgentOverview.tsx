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

import {
  Box,
  Typography,
  alpha,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  IconButton,
  Tooltip,
  TextField,
  InputAdornment,
  Collapse,
  ButtonBase,
} from "@mui/material";
import {
  Timeline,
  ExpandMore,
  ContentCopy,
  KeyboardArrowDown,
} from "@mui/icons-material";
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { CodeBlock } from "@agent-management-platform/shared-component";

export const ExternalAgentOverview = () => {
  const { agentId } = useParams<{ agentId: string }>();
  const [observeExpanded, setObserveExpanded] = useState(() => {
    const observeStateStr = localStorage.getItem("observeExpanded");
    const state = JSON.parse(observeStateStr ?? "{}");
    try {
      return state?.[agentId ?? "default"] !== false;
    } catch (error) {
      console.error(error);
    }
    return true;
  });

  const [telemetryExpanded, setTelemetryExpanded] = useState(false);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  useEffect(() => {
    const observeStateStr = localStorage.getItem("observeExpanded");
    try {
      const state = JSON.parse(observeStateStr ?? "{}");
      localStorage.setItem(
        "observeExpanded",
        JSON.stringify(
          {
            ...state,
            [agentId ?? "default"]: false,
          },
          null,
          2
        )
      );
    } catch (error) {
      console.error(error);
    }
  }, [observeExpanded, agentId]);

  // Sample instrumentation config - these would come from props or API
  const instrumentationUrl = "http://localhost:21893";
  const apiKey = "00000000-0000-0000-0000-000000000000";

  const handleCopy = async (text: string, field: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    } catch {
      // Failed to copy - silently fail
    }
  };

  return (
    <Box display="flex" flexDirection="column" gap={3} pt={2}>
      <Accordion
        expanded={observeExpanded}
        onChange={(_, isExpanded) => setObserveExpanded(isExpanded)}
      >
        <AccordionSummary expandIcon={<ExpandMore />}>
          <Box display="flex" gap={2} alignItems="center" flex={1}>
            <Timeline fontSize="large" />
            <Box flex={1}>
              <Box display="flex" alignItems="center" gap={1} mb={0.5}>
                <Typography variant="h5" fontWeight={600}>
                  Observe Your Agent
                </Typography>
              </Box>
              <Typography
                variant="body2"
                color="text.secondary"
                maxWidth="700px"
              >
                Get complete visibility into your agent&apos;s behavior with
                distributed tracing, real-time metrics, and structured logs.
              </Typography>
            </Box>
          </Box>
        </AccordionSummary>

        <AccordionDetails sx={{ p: 3, pt: 0 }}>
          {/* Instrumentation Configuration */}
          {/* Quick Setup Guide */}
          <Box
            mb={1}
            display="flex"
            flexDirection="column"
            border="1px solid"
            borderColor="divider"
            p={2}
            borderRadius={2}
          >
            <Box
              display="flex"
              justifyContent="space-between"
              alignItems="center"
              mb={1}
            >
              <Box>
                <Typography variant="h6" fontWeight={600} gutterBottom>
                  Quick Setup
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Auto-instrument with environment variables
                </Typography>
              </Box>
            </Box>

            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box display="flex" gap={2} flexDirection="column">
                <Box display="flex" alignItems="center" gap={1}>
                  <Box
                    sx={{
                      width: 20,
                      height: 20,
                      borderRadius: "50%",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      bgcolor: (theme) =>
                        alpha(theme.palette.primary.main, 0.5),
                      color: "primary.contrastText",
                      fontWeight: 600,
                      flexShrink: 0,
                    }}
                  >
                    <Typography variant="body2" fontWeight={600}>
                      1
                    </Typography>
                  </Box>
                  <Typography variant="body2" fontWeight={600}>
                    Install agent instrumentation package
                  </Typography>
                </Box>
                <Box>
                  <CodeBlock
                    code="pip install agent-instrumentation"
                    language="bash"
                    fieldId="install"
                  />
                </Box>
              </Box>

              <Box display="flex" gap={2} flexDirection="column">
                <Box display="flex" alignItems="center" gap={1}>
                  <Box
                    sx={{
                      width: 20,
                      height: 20,
                      borderRadius: "50%",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      bgcolor: (theme) =>
                        alpha(theme.palette.primary.main, 0.5),
                      color: "primary.contrastText",
                      fontWeight: 600,
                      flexShrink: 0,
                    }}
                  >
                    <Typography variant="body2" fontWeight={600}>
                      2
                    </Typography>
                  </Box>
                  <Typography variant="body2" fontWeight={600}>
                    Set environment variables
                  </Typography>
                </Box>
                <Box>
                  <CodeBlock
                    code={`export AMP_APP_NAME="${agentId}"
export AMP_OTEL_EXPORTER_OTLP_ENDPOINT="${instrumentationUrl}"
export AMP_API_KEY="${apiKey}"`}
                    language="bash"
                    fieldId="env"
                  />
                </Box>
              </Box>

              <Box display="flex" gap={2} flexDirection="column">
                <Box display="flex" alignItems="center" gap={1}>
                  <Box
                    sx={{
                      width: 20,
                      height: 20,
                      borderRadius: "50%",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      bgcolor: (theme) =>
                        alpha(theme.palette.primary.main, 0.5),
                      color: "primary.contrastText",
                      fontWeight: 600,
                      flexShrink: 0,
                    }}
                  >
                    <Typography variant="body2" fontWeight={600}>
                      3
                    </Typography>
                  </Box>
                  <Typography variant="body2" fontWeight={600}>
                    Run your agent with auto-instrumentation
                  </Typography>
                </Box>
                <Box>
                  <CodeBlock
                    code="agent-trace <run_command>"
                    language="bash"
                    fieldId="run"
                  />
                </Box>
              </Box>
              <Box
                sx={{
                  display: "flex",
                  gap: 1,
                  p: 2,
                  bgcolor: (theme) => alpha(theme.palette.primary.main, 0.1),
                  borderRadius: 2,
                  mt: 1,
                }}
              >
                <Box>
                  <Typography variant="body2" fontWeight={600}>
                    No code changes required!
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Auto-instrumentation detects and instruments popular
                    frameworks like LangChain, CrewAI, AutoGen, and LlamaIndex
                    automatically.
                  </Typography>
                </Box>
              </Box>
            </Box>
          </Box>
          <Box
            mb={1}
            display="flex"
            flexDirection="column"
            border="1px solid"
            borderColor="divider"
            borderRadius={2}
          >
            <ButtonBase
              onClick={() => setTelemetryExpanded(!telemetryExpanded)}
              sx={{
                width: "100%",
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                p: 2,
                textAlign: "left",
                borderRadius: 1,
                "&:hover": {
                  bgcolor: (theme) => alpha(theme.palette.action.hover, 0.02),
                },
              }}
            >
              <Box>
                <Typography variant="h6" fontWeight={600} gutterBottom>
                  Telemetry Configuration
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  OpenTelemetry collector endpoint and authentication
                  credentials for publishing traces.
                </Typography>
              </Box>
              <KeyboardArrowDown
                fontSize="small"
                sx={{
                  transform: telemetryExpanded
                    ? "rotate(180deg)"
                    : "rotate(0deg)",
                  transition: "transform 0.3s",
                  color: "text.secondary",
                }}
              />
            </ButtonBase>

            <Collapse in={telemetryExpanded}>
              <Box display="flex" flexDirection="column" gap={1} p={2} pt={0}>
                <TextField
                  label="Collector URL"
                  value={instrumentationUrl}
                  fullWidth
                  variant="outlined"
                  slotProps={{
                    input: {
                      readOnly: true,
                      endAdornment: (
                        <InputAdornment position="end">
                          <Tooltip
                            title={
                              copiedField === "url" ? "Copied!" : "Copy URL"
                            }
                          >
                            <IconButton
                              onClick={() =>
                                handleCopy(instrumentationUrl, "url")
                              }
                              edge="end"
                              size="small"
                            >
                              <ContentCopy fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        </InputAdornment>
                      ),
                    },
                  }}
                />

                <TextField
                  label="API Key"
                  value={apiKey}
                  fullWidth
                  variant="outlined"
                  slotProps={{
                    input: {
                      readOnly: true,
                      endAdornment: (
                        <InputAdornment position="end">
                          <Tooltip
                            title={
                              copiedField === "apiKey"
                                ? "Copied!"
                                : "Copy API Key"
                            }
                          >
                            <IconButton
                              onClick={() => handleCopy(apiKey, "apiKey")}
                              edge="end"
                              size="small"
                            >
                              <ContentCopy fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        </InputAdornment>
                      ),
                    },
                  }}
                />
              </Box>
            </Collapse>
          </Box>
        </AccordionDetails>
      </Accordion>
    </Box>
  );
};
