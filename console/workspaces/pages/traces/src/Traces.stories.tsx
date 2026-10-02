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

import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import type { TraceFilters, TraceOverview } from '@agent-management-platform/types';
// Direct file imports: the subComponents index pulls in api-client via TraceDetails.
import { TraceFilterBar } from './subComponents/TraceFilterBar';
import { TracesView } from './subComponents/TracesView';
import type { TraceColumn } from './traceColumns';

const sampleTraces = [
  {
    traceId: 'trace-1',
    rootSpanName: 'invoke_agent',
    input: 'What is the weather in Colombo?',
    output: 'Error: upstream timeout',
    startTime: '2026-10-01T10:42:00Z',
    durationInNanos: 7_200_000_000,
    spanCount: 24,
    status: { errorCount: 2 },
    tokenUsage: { inputTokens: 3100, outputTokens: 1020, totalTokens: 4120 },
    conversationId: 'conv-7f3a9c2e-41d8-4b6f-9a0e-5c2d1e8b7f60',
    models: ['gpt-4o', 'gpt-4o-mini'],
  },
  {
    traceId: 'trace-2',
    rootSpanName: 'invoke_agent',
    input: 'Search the web for agent tracing',
    output: 'Error: tool call failed',
    startTime: '2026-10-01T10:31:00Z',
    durationInNanos: 5_800_000_000,
    spanCount: 21,
    status: { errorCount: 1 },
    tokenUsage: { inputTokens: 2100, outputTokens: 800, totalTokens: 2900 },
    conversationId: 'conv-7f3a9c2e-41d8-4b6f-9a0e-5c2d1e8b7f60',
    models: ['gpt-4o'],
  },
] as TraceOverview[];

interface FilteredTracesProps {
  initialFilters: TraceFilters;
  traces: TraceOverview[];
  hasOlder?: boolean;
  truncated?: boolean;
  lookedBackTo?: string;
  visibleColumns?: TraceColumn[];
}

// The filter bar over the list, with filters held in local state instead of the URL.
function FilteredTraces({
  initialFilters,
  traces,
  hasOlder,
  truncated,
  lookedBackTo,
  visibleColumns,
}: FilteredTracesProps) {
  const [filters, setFilters] = useState(initialFilters);
  return (
    <>
      <TraceFilterBar filters={filters} onChange={setFilters} />
      <TracesView
        traces={traces}
        selectedTrace={null}
        hasOlder={hasOlder}
        hasActiveFilters={Object.keys(filters).length > 0}
        truncated={truncated}
        lookedBackTo={lookedBackTo}
        visibleColumns={visibleColumns}
        onTraceSelect={() => undefined}
        onLoadOlder={() => undefined}
        onLoadNewer={() => undefined}
        onConversationSelect={(conversationId) => setFilters({ ...filters, conversationId })}
      />
    </>
  );
}

const meta: Meta<typeof FilteredTraces> = {
  title: 'Pages/Traces/Filters',
  component: FilteredTraces,
  parameters: {
    layout: 'padded',
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const NoFilters: Story = {
  args: {
    initialFilters: {},
    traces: sampleTraces,
  },
};

export const SeveralActiveFilters: Story = {
  args: {
    initialFilters: {
      status: 'error',
      minDurationMs: 5000,
      minTokens: 1000,
      model: 'gpt-4o',
      conversationId: 'conv-7f3a',
    },
    traces: sampleTraces,
  },
};

export const EmptyFilteredPageWithOlder: Story = {
  args: {
    initialFilters: { status: 'error', minSpanCount: 50 },
    traces: [],
    hasOlder: true,
  },
};

export const ModelColumnAndCapNotice: Story = {
  args: {
    initialFilters: { status: 'error' },
    traces: sampleTraces,
    visibleColumns: ['conversation', 'model'],
    hasOlder: true,
    truncated: true,
    lookedBackTo: '2026-10-01T08:14:00Z',
  },
};
