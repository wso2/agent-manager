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

import { Box, Typography, Select, MenuItem, SelectChangeEvent, Stack } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { SetupStep } from "./SetupStep";
import { TokenGenerationStep } from "./TokenGenerationStep";
import { useState } from "react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";

type Language = "python" | "ballerina";

interface InstrumentationDrawerProps {
  open: boolean;
  onClose: () => void;
  agentId: string;
  orgName: string;
  projName: string;
  agentName: string;
  environment?: string;
  instrumentationUrl: string;
  apiKey: string;
  componentUid?: string;
  environmentUid?: string;
}

export const InstrumentationDrawer = ({
  open,
  onClose,
  orgName,
  projName,
  agentName,
  environment,
  instrumentationUrl,
  apiKey,
  componentUid,
  environmentUid,
}: InstrumentationDrawerProps) => {
  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  const [selectedLanguage, setSelectedLanguage] = useState<Language>("python");
  
  // Use generated key if available, otherwise use the passed apiKey
  const effectiveApiKey = generatedApiKey || apiKey;

  const handleLanguageChange = (event: SelectChangeEvent<Language>) => {
    setSelectedLanguage(event.target.value as Language);
  };

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={700}>
      <DrawerHeader
        icon={<Settings size={24} />}
        title="Setup Agent"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="h5">Zero-code Instrumentation Guide</Typography>
          <Stack direction="row" alignItems="center" gap={1}>
            <Typography variant="body2">Language:</Typography>
          <Select
            value={selectedLanguage}
            onChange={handleLanguageChange}
          >
            <MenuItem value="python">Python</MenuItem>
            <MenuItem value="ballerina">Ballerina</MenuItem>
          </Select>
          </Stack>
        </Stack>
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
            pt: 1,
            width: "100%",
          }}
        >
          {selectedLanguage === "python" ? (
            <>
              <SetupStep
                stepNumber={1}
                title="Install AMP Instrumentation Package"
                code="pip install amp-instrumentation"
                language="bash"
                fieldId="install"
                description="Provides the ability to instrument your agent and export traces."
              />
              <TokenGenerationStep
                stepNumber={2}
                orgName={orgName}
                projName={projName}
                agentName={agentName}
                environment={environment}
                onTokenGenerated={setGeneratedApiKey}
              />
              <SetupStep
                stepNumber={3}
                title="Set environment variables"
                code={`export AMP_OTEL_ENDPOINT="${instrumentationUrl}"
export AMP_AGENT_API_KEY="${effectiveApiKey}"`}
                language="bash"
                fieldId="env"
                description="Sets the agent endpoint and agent-specific API key so traces can be exported securely."
              />
              <SetupStep
                stepNumber={4}
                title="Run Agent with Instrumentation Enabled"
                code="amp-instrument <run_command>"
                language="bash"
                fieldId="run"
                description="Replace <run_command> with your agent's start command. For example: amp-instrument python app.py"
              />
            </>
          ) : (
            <>
              <SetupStep
                stepNumber={1}
                title="Import Amp Module"
                code="import ballerinax/amp as _;"
                language="ballerina"
                fieldId="import"
                description="Add the import to your Ballerina program."
              />
              <SetupStep
                stepNumber={2}
                title="Add the following configuration to Ballerina.toml"
                code={`[build-options]
observabilityIncluded = true`}
                language="toml"
                fieldId="ballerina-toml"
                description="Ensure the following configuration is present when building the program."
              />
              <SetupStep
                stepNumber={3}
                title="Update Config.toml"
                code={`[ballerina.observe]
tracingEnabled = true
tracingProvider = "amp"`}
                language="toml"
                fieldId="config-toml"
                description="Enable tracing and set the provider to Amp."
              />
              <TokenGenerationStep
                stepNumber={4}
                orgName={orgName}
                projName={projName}
                agentName={agentName}
                environment={environment}
                onTokenGenerated={setGeneratedApiKey}
              />
              <SetupStep
                stepNumber={5}
                title="Set Environment Variables"
                code={`export BAL_CONFIG_VAR_BALLERINAX_AMP_OTELENDPOINT="${instrumentationUrl}"
export BAL_CONFIG_VAR_BALLERINAX_AMP_APIKEY="${effectiveApiKey}"
export BAL_CONFIG_VAR_BALLERINAX_AMP_COMPONENTUID="${componentUid || ''}"
export BAL_CONFIG_VAR_BALLERINAX_AMP_ENVIRONMENTUID="${environmentUid || ''}"`}
                language="bash"
                fieldId="env-vars"
                description="Configure the Amp connection using environment variables."
              />
            </>
          )}
        </Box>
      </DrawerContent>
    </DrawerWrapper>
  );
};
