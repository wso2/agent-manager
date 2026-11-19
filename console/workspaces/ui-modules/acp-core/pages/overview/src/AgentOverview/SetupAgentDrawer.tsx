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

import { Settings } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { TokenGenerationStep } from "./TokenGenerationStep";

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

// Issues an external agent's API key. The tracing guide that used to sit
// here comes back with the observability module.
export const SetupAgentDrawer = ({
  open,
  onClose,
  orgName,
  projName,
  agentName,
  environment,
  autoGenerate = false,
}: SetupAgentDrawerProps) => (
  <DrawerWrapper open={open} onClose={onClose} maxWidth={700}>
    <DrawerHeader icon={<Settings size={24} />} title="Setup Agent" onClose={onClose} />
    <DrawerContent>
      <TokenGenerationStep
        stepNumber={1}
        orgName={orgName}
        projName={projName}
        agentName={agentName}
        environment={environment}
        autoGenerate={autoGenerate}
      />
    </DrawerContent>
  </DrawerWrapper>
);
