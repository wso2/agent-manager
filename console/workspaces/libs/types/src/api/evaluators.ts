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

import { type ListQuery, type OrgPathParams } from "./common";
import { type EvaluationLevel } from "./monitors";

export interface EvaluatorConfigParam {
  key: string;
  type: string;
  description: string;
  required: boolean;
  default?: unknown;
  min?: number;
  max?: number;
  enumValues?: string[];
}

export interface EvaluatorResponse {
  id: string;
  identifier: string;
  displayName: string;
  description: string;
  version: string;
  provider: string;
  level?: EvaluationLevel;
  tags: string[];
  isBuiltin: boolean;
  configSchema: EvaluatorConfigParam[];
  createdAt: string;
  updatedAt: string;
}

export interface EvaluatorListResponse {
  evaluators: EvaluatorResponse[];
  total: number;
  limit: number;
  offset: number;
}

export interface EvaluatorListQuery extends ListQuery {
  tags?: string[];
  search?: string;
  provider?: string;
}

export type ListEvaluatorsPathParams = OrgPathParams;

export interface GetEvaluatorPathParams extends OrgPathParams {
  evaluatorId: string | undefined;
}
