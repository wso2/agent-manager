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

import { Box, Typography } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { SetupStep } from "./SetupStep";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";

interface InstrumentationDrawerProps {
  open: boolean;
  onClose: () => void;
  agentId: string;
  instrumentationUrl: string;
  apiKey: string;
  traceAttributes: string;
}

export const InstrumentationDrawer = ({
  open,
  onClose,
  agentId,
  instrumentationUrl,
  apiKey,
  traceAttributes,
}: InstrumentationDrawerProps) => {
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
          <SetupStep
            stepNumber={2}
            title="Set environment variables"
            code={`export AMP_AGENT_NAME="${agentId}"
export AMP_OTEL_ENDPOINT="${instrumentationUrl}"
export AMP_AGENT_API_KEY="${apiKey}"
export AMP_TRACE_ATTRIBUTES="${traceAttributes}"`}
            language="bash"
            fieldId="env"
            description="Sets the agent endpoint and agent-specific API key so traces can be exported securely."
          />
          <SetupStep
            stepNumber={3}
            title="Run Agent with Instrumentation Enabled"
            code="amp-instrument <run_command>"
            language="bash"
            fieldId="run"
            description="Look at the code-block in the screenshot, that way we can give a default command but also tell in a comment that user should replace it with what makes sense for them."
          />
        </Box>
      </DrawerContent>
    </DrawerWrapper>
  );
};
