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

import { useState } from "react";
import { Box, MenuItem, Select, SelectChangeEvent, Stack, Typography } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { moduleBase, useUIManifest } from "@agent-management-platform/api-client";
import { globalConfig } from "@agent-management-platform/types";
import { SetupStep } from "./SetupStep";
import { TokenGenerationStep } from "./TokenGenerationStep";

type Language = "python" | "other";

/** The OTLP endpoint agents send traces to: the observability module, beside the API. */
export const traceEndpoint = (apiBaseUrl: string, origin: string): string => {
  const base = /^https?:\/\//.test(apiBaseUrl) ? apiBaseUrl : `${origin}${apiBaseUrl}`;
  return `${base.replace(/\/+$/, "")}${moduleBase("observability")}/otel`;
};

interface SetupAgentDrawerProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
  environment?: string;
  // Mint a token automatically on first open (post-creation convenience). Guarded per session.
  autoGenerate?: boolean;
}

// Issues an external agent's API key and, while the observability module is
// active, shows how to send traces to it with that key.
export const SetupAgentDrawer = ({
  open,
  onClose,
  orgName,
  projName,
  agentName,
  environment,
  autoGenerate = false,
}: SetupAgentDrawerProps) => {
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [language, setLanguage] = useState<Language>("python");
  const { data: manifest } = useUIManifest();
  const tracing = manifest?.modules.some((m) => m.name === "observability") ?? false;
  const endpoint = tracing ? traceEndpoint(globalConfig?.apiBaseUrl ?? "", window.location.origin) : "";
  // A placeholder until a key is generated, so the examples never show "null".
  const key = apiKey ?? "<your-api-key>";

  const tokenStep = (stepNumber: number) => (
    <TokenGenerationStep
      stepNumber={stepNumber}
      orgName={orgName}
      projName={projName}
      agentName={agentName}
      environment={environment}
      onTokenGenerated={setApiKey}
      autoGenerate={autoGenerate}
    />
  );

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={700}>
      <DrawerHeader icon={<Settings size={24} />} title="Setup Agent" onClose={onClose} />
      <DrawerContent>
        {!tracing ? (
          tokenStep(1)
        ) : (
          <>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Typography variant="h5">
                {language === "python"
                  ? "Zero-code Instrumentation Guide"
                  : "OpenTelemetry Export Guide"}
              </Typography>
              <Stack direction="row" alignItems="center" gap={1}>
                <Typography id="setup-agent-language" variant="body2">Language:</Typography>
                <Select
                  labelId="setup-agent-language"
                  value={language}
                  onChange={(e: SelectChangeEvent<Language>) =>
                    setLanguage(e.target.value as Language)
                  }
                >
                  <MenuItem value="python">Python</MenuItem>
                  <MenuItem value="other">Other</MenuItem>
                </Select>
              </Stack>
            </Stack>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 3, pt: 1, width: "100%" }}>
              {language === "python" ? (
                <>
                  <SetupStep
                    stepNumber={1}
                    title="Install AMP Instrumentation Package"
                    code="pip install amp-instrumentation"
                    fieldId="install"
                    description="Provides the ability to instrument your agent and export traces."
                  />
                  {tokenStep(2)}
                  <SetupStep
                    stepNumber={3}
                    title="Set environment variables"
                    code={`export AMP_OTEL_ENDPOINT="${endpoint}"
export AMP_AGENT_API_KEY="${key}"`}
                    fieldId="env"
                    description="Sends the agent's traces to AMP, authenticated with its API key."
                  />
                  <SetupStep
                    stepNumber={4}
                    title="Run Agent with Instrumentation Enabled"
                    code="amp-instrument <run_command>"
                    fieldId="run"
                    description="Replace <run_command> with your agent's start command. For example: amp-instrument python app.py"
                  />
                </>
              ) : (
                <>
                  {tokenStep(1)}
                  <SetupStep
                    stepNumber={2}
                    title="Configure Your OTLP Exporter"
                    code={`export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="${endpoint}/v1/traces"
export OTEL_EXPORTER_OTLP_TRACES_PROTOCOL="http/protobuf"
export OTEL_EXPORTER_OTLP_TRACES_HEADERS="x-amp-api-key=${key}"`}
                    fieldId="otlp-env"
                    description="Point any OpenTelemetry OTLP/HTTP exporter at this endpoint with the x-amp-api-key header. Emit spans using the OpenTelemetry GenAI semantic conventions."
                  />
                </>
              )}
            </Box>
          </>
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
};
