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

import { describe, expect, it } from 'vitest';
import type { AgentCardResponse } from '@agent-management-platform/types';
import { cardState } from './cardState';

const ready = { source: 'platform', status: 'fetched', sourceUrl: '', lastError: '', card: { name: 'Orders' } } as AgentCardResponse;
const pending = { source: 'platform', status: 'pending', sourceUrl: '', lastError: '' } as AgentCardResponse;

describe('cardState', () => {
  it.each([
    [{ data: ready, isLoading: false, isError: false }, 'ready'],
    [{ data: ready, isLoading: false, isError: true }, 'ready'],
    [{ data: pending, isLoading: false, isError: false }, 'noBody'],
    [{ data: { ...pending, card: null as unknown as undefined }, isLoading: false, isError: false }, 'noBody'],
    [{ data: null, isLoading: false, isError: false }, 'absent'],
    [{ data: undefined, isLoading: false, isError: true }, 'error'],
    [{ data: undefined, isLoading: true, isError: false }, 'loading'],
    [{ data: undefined, isLoading: false, isError: false }, 'loading'],
  ])('maps %j to %s', (query, state) => {
    expect(cardState(query).state).toBe(state);
  });

  it('carries the body when ready', () => {
    expect(cardState({ data: ready, isLoading: false, isError: false })).toEqual({ state: 'ready', response: ready, card: { name: 'Orders' } });
  });
});
