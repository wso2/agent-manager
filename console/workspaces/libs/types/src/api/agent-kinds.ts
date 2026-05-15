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

import type { AgentPathParams, ListQuery, OrgPathParams } from './common';

// ============================================
// Shared sub-types
// ============================================

export interface AgentKindConfigSchemaItem {
  name: string;
  description?: string;
  isSecret: boolean;
  isMandatory: boolean;
  defaultValue?: string | null;
}

// ============================================
// Response types
// ============================================

export interface AgentKindVersionResponse {
  version: string;
  buildName?: string;
  imageId: string;
  sourceAgentName: string;
  sourceProjectName: string;
  agentSubType?: 'chat-api' | 'custom-api';
  configSchema: AgentKindConfigSchemaItem[];
  createdAt: string;
}

export interface AgentKindResponse {
  uuid: string;
  name: string;
  displayName: string;
  description?: string;
  organizationName: string;
  kind: 'AgentKind';
  latestVersion?: string;
  versions: AgentKindVersionResponse[];
  createdAt: string;
  updatedAt?: string;
}

export interface AgentKindListResponse {
  kinds: AgentKindResponse[];
  total: number;
  limit: number;
  offset: number;
}

// ============================================
// Request types
// ============================================

export interface UpdateAgentKindRequest {
  displayName: string;
  description?: string;
}

export interface AddAgentKindVersionRequest {
  version: string;
  buildName: string;
  sourceAgentName: string;
  sourceProjectName: string;
  configSchema: AgentKindConfigSchemaItem[];
}

export interface PublishAgentKindRequest {
  kindName: string;
  kindDisplayName?: string;
  kindDescription?: string;
  version: string;
  buildName: string;
  configSchema: AgentKindConfigSchemaItem[];
}

// ============================================
// Path params
// ============================================

export type ListAgentKindsPathParams = OrgPathParams;
export type GetAgentKindPathParams = OrgPathParams & { kindName: string };
export type UpdateAgentKindPathParams = OrgPathParams & { kindName: string };
export type DeleteAgentKindPathParams = OrgPathParams & { kindName: string };
export type ListAgentKindVersionsPathParams = OrgPathParams & { kindName: string };
export type AddAgentKindVersionPathParams = OrgPathParams & { kindName: string };
export type GetAgentKindVersionPathParams = OrgPathParams & {
  kindName: string;
  versionTag: string;
};
export type DeleteAgentKindVersionPathParams = OrgPathParams & {
  kindName: string;
  versionTag: string;
};
export type PublishAgentKindPathParams = AgentPathParams;
export type ListKindAgentsPathParams = OrgPathParams & { kindName: string };

// ============================================
// Query params
// ============================================

export type ListAgentKindsQuery = ListQuery;
