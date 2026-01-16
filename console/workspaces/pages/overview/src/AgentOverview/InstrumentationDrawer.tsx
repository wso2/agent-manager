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

import { Box, Typography } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { SetupStep } from "./SetupStep";
import { TokenGenerationStep } from "./TokenGenerationStep";
import { useState } from "react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";

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
  traceAttributes: string;
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
  traceAttributes,
}: InstrumentationDrawerProps) => {
  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  
  // Use generated key if available, otherwise use the passed apiKey
  const effectiveApiKey = generatedApiKey || apiKey;

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={700}>
      <DrawerHeader
        icon={<Settings size={24} />}
        title="Setup Agent"
        onClose={onClose}
      />
      <DrawerContent>
        <Typography variant="h5">Zero-code Instrumentation Guide</Typography>
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
            pt: 1,
            width: "100%",
          }}
        >
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
export AMP_AGENT_API_KEY="${effectiveApiKey}"
export AMP_TRACE_ATTRIBUTES="${traceAttributes}"`}
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
        </Box>
      </DrawerContent>
    </DrawerWrapper>
  );
};
