/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Chip } from "@wso2/oxygen-ui";
import type { AgentType, Provisioning } from "@agent-management-platform/types";
import { displayAgentSubType, displayProvisionTypes } from "@agent-management-platform/views";

interface AgentTypeChipsProps {
  provisioning?: Provisioning;
  agentType?: AgentType;
  kindName?: string;
  /** Resolved display name for `kindName`; falls back to the raw name when omitted. */
  kindDisplayName?: string;
}

const CHIP_SX = { flexShrink: 0, height: 20, fontSize: "0.6875rem" };

export function AgentTypeChips(
  { provisioning, agentType, kindName, kindDisplayName }: AgentTypeChipsProps,
) {
  return (
    <>
      {provisioning?.type !== "internal" ? (
        <Chip
          label={displayProvisionTypes(provisioning?.type)}
          size="small"
          variant="outlined"
          sx={CHIP_SX}
        />
      ) : (
        agentType?.subType && (
          <Chip
            label={displayAgentSubType(agentType.subType)}
            size="small"
            variant="outlined"
            sx={CHIP_SX}
          />
        )
      )}
      {kindName && (
        <Chip
          label={`Kind: ${kindDisplayName ?? kindName}`}
          size="small"
          variant="outlined"
          sx={CHIP_SX}
        />
      )}
    </>
  );
}
