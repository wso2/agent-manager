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

import { type AgentPathParams } from './common';

export interface CreateAgentAPIKeyRequest {
  name?: string;
  displayName?: string;
  expiresAt?: string;
}

export interface CreateAgentAPIKeyResponse {
  status: string;
  message: string;
  keyId?: string;
  apiKey?: string;
}

export interface RotateAgentAPIKeyRequest {
  displayName?: string;
  expiresAt?: string;
}

export interface RotateAgentAPIKeyResponse {
  status: string;
  message: string;
  keyId?: string;
  apiKey?: string;
}

export interface AgentAPIKeyListItem {
  uuid: string;
  name: string;
  maskedApiKey: string;
  status: string;
  createdAt: string;
  expiresAt?: string;
}

export type AgentAPIKeyListResponse = AgentAPIKeyListItem[];

export interface AgentAPIKeyPathParams extends AgentPathParams {
  keyName: string | undefined;
}

export type CreateAgentAPIKeyPathParams = AgentPathParams;
export type RotateAgentAPIKeyPathParams = AgentAPIKeyPathParams;
export type RevokeAgentAPIKeyPathParams = AgentAPIKeyPathParams;
export type ListAgentAPIKeysPathParams = AgentPathParams;
