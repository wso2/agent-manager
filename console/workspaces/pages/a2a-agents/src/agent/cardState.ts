/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { AgentCardResponse } from '@agent-management-platform/types';

export interface CardQueryResult {
  data: AgentCardResponse | null | undefined;
  isLoading: boolean;
  isError: boolean;
}

export type CardState =
  | { state: 'ready'; response: AgentCardResponse; card: Record<string, unknown> }
  | { state: 'noBody'; response: AgentCardResponse }
  | { state: 'absent' }
  | { state: 'error' }
  | { state: 'loading' };

function cardBody(response: AgentCardResponse): Record<string, unknown> | undefined {
  const { card } = response;
  const isObject = typeof card === 'object' && card !== null && !Array.isArray(card);
  return isObject ? (card as Record<string, unknown>) : undefined;
}

export function cardState({ data, isError }: CardQueryResult): CardState {
  if (data) {
    const card = cardBody(data);
    return card ? { state: 'ready', response: data, card } : { state: 'noBody', response: data };
  }
  if (data === null) return { state: 'absent' };
  if (isError) return { state: 'error' };
  return { state: 'loading' };
}
