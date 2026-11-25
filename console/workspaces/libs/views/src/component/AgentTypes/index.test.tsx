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

import type { ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import {
  AgentTypesProvider,
  useAgentTypes,
  useAgentTypesState,
  type AgentTypeOption,
} from '../AgentTypes';
import {
  ResourceSummariesProvider,
  useResourceSummaries,
  type ResourceSummaryOption,
} from '../ResourceSummaries';

const agentType = { key: 'p:external', pluginId: 'p', title: 'External' } as unknown as AgentTypeOption;
const summary = { key: 'p:agents', pluginId: 'p', title: 'Agents' } as unknown as ResourceSummaryOption;

describe('agent type and resource summary contexts', () => {
  it('are empty and settled outside a plugin host', () => {
    expect(renderHook(() => useAgentTypes()).result.current).toEqual([]);
    expect(renderHook(() => useAgentTypesState()).result.current).toEqual({
      agentTypes: [],
      isPending: false,
    });
    expect(renderHook(() => useResourceSummaries()).result.current).toEqual([]);
  });

  it('expose what the host provides', () => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <AgentTypesProvider value={{ agentTypes: [agentType], isPending: true }}>
        <ResourceSummariesProvider value={[summary]}>{children}</ResourceSummariesProvider>
      </AgentTypesProvider>
    );
    const { result } = renderHook(
      () => ({
        types: useAgentTypes(),
        state: useAgentTypesState(),
        summaries: useResourceSummaries(),
      }),
      { wrapper },
    );
    expect(result.current.types).toEqual([agentType]);
    expect(result.current.state.isPending).toBe(true);
    expect(result.current.summaries).toEqual([summary]);
  });
});
