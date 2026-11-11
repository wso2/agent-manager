/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import type { PageMetadata } from "@agent-management-platform/types";
import { AgentIdentityOrganization } from "./AgentIdentity.Organization";
import { agentIdentityMetadata } from "./metadata";
import { AgentIdComponent } from "./agentId/AgentId.Component";

export const agentIdentityMetaData: PageMetadata = {
  title: agentIdentityMetadata.title,
  description: "Agent identities for agent authentication",
  icon: agentIdentityMetadata.icon,
  path: "/agent-identity",
  component: AgentIdentityOrganization,
  levels: {
    organization: AgentIdentityOrganization,
  },
};

export const agentIdMetaData: PageMetadata = {
  title: "Agent ID",
  description: "Client credentials, roles & groups, and OAuth2 endpoints for an agent's identity",
  icon: ShieldCheck,
  path: "/agent-id",
  component: AgentIdComponent,
};

export { AgentIdentityOrganization, agentIdentityMetadata, AgentIdComponent };

export default AgentIdentityOrganization;
