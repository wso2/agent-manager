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
import { userEvent, within } from '@storybook/test';
import type { TraceFilters, TraceOverview } from '@agent-management-platform/types';
// Direct file imports: the subComponents index pulls in api-client via TraceDetails.
import { TraceFilterBar } from './subComponents/TraceFilterBar';
import { TracesView } from './subComponents/TracesView';
import { hasScoreFilter } from './traceFilters';

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
  },
] as TraceOverview[];

const sampleEvaluators = ['Accuracy', 'Helpfulness', 'Tool Use'];

interface FilteredTracesProps {
  initialFilters: TraceFilters;
  // Opens the Filters drawer on the initial filters.
  initialOpen?: boolean;
  traces: TraceOverview[];
  hasMore?: boolean;
  isLoadingMore?: boolean;
  loadError?: Error | null;
  truncated?: boolean;
  lookedBackTo?: string;
}

// The filter bar over the list, with filters held in local state instead of the URL.
function FilteredTraces({
  initialFilters,
  initialOpen = false,
  traces,
  hasMore,
  isLoadingMore,
  loadError,
  truncated,
  lookedBackTo,
}: FilteredTracesProps) {
  const [filters, setFilters] = useState(initialFilters);
  const [open, setOpen] = useState(initialOpen);
  return (
    <TraceFilterBar
      filters={filters}
      onChange={setFilters}
      open={open}
      onOpenChange={setOpen}
      onTraceSearch={() => undefined}
      evaluators={sampleEvaluators}
    >
      <TracesView
        traces={traces}
        selectedTrace={null}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
        loadError={loadError}
        hasActiveFilters={Object.keys(filters).length > 0}
        hasScoreFilter={hasScoreFilter(filters)}
        scoreEvaluator={filters.evaluator}
        truncated={truncated}
        lookedBackTo={lookedBackTo}
        onTraceSelect={() => setOpen(false)}
        onLoadMore={async () => undefined}
        onConversationSelect={(conversationId) => setFilters({ ...filters, conversationId })}
      />
    </TraceFilterBar>
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

// Tool, MCP server and score chips, and the Score column labelled by evaluator.
export const ToolMcpAndScoreFilters: Story = {
  args: {
    initialFilters: {
      tool: 'search_web',
      toolError: true,
      mcpServer: 'github',
      evaluator: 'Accuracy',
      minScore: 0.1,
      maxScore: 0.5,
    },
    traces: [{ ...sampleTraces[1], score: { score: 0.2, totalCount: 1, skippedCount: 0 } }],
  },
};

// The Filters drawer open over the list, as in optionD.png.
export const FiltersDrawerOpen: Story = {
  args: {
    initialFilters: { status: 'error', minDurationMs: 5000, tool: 'reset_password' },
    initialOpen: true,
    traces: sampleTraces,
  },
};

// A pasted 2500 ms shows as its own pill; Custom opens the field prefilled with 2.5 s.
export const CustomValue: Story = {
  args: {
    initialFilters: { minDurationMs: 2500 },
    initialOpen: true,
    traces: sampleTraces,
  },
  play: async ({ canvasElement }) => {
    const latency = within(canvasElement).getByRole('group', { name: 'Latency at least' });
    await userEvent.click(within(latency).getByRole('button', { name: 'Custom' }));
  },
};

// Both score bounds with an evaluator; Score at most 25% would cross Score at least 50%.
export const ScoreRangeWithEvaluator: Story = {
  args: {
    initialFilters: { evaluator: 'Accuracy', minScore: 0.5, maxScore: 0.75 },
    initialOpen: true,
    traces: [{ ...sampleTraces[0], score: { score: 0.6, totalCount: 2, skippedCount: 0 } }],
  },
};

// Scrolled to the end of the list while the next page loads.
export const MidScroll: Story = {
  args: {
    initialFilters: {},
    traces: sampleTraces,
    hasMore: true,
    isLoadingMore: true,
  },
};

// A filtered page with no matches doesn't auto-load; the next page loads on click.
export const PausedOnEmptyFilteredPage: Story = {
  args: {
    initialFilters: { status: 'error', minSpanCount: 50 },
    traces: [],
    hasMore: true,
    lookedBackTo: '2026-10-01T08:14:00Z',
  },
};

export const LoadError: Story = {
  args: {
    initialFilters: {},
    traces: sampleTraces,
    hasMore: true,
    loadError: new Error('upstream timeout'),
  },
};

export const CapNotice: Story = {
  args: {
    initialFilters: { status: 'error' },
    traces: sampleTraces,
    hasMore: true,
    truncated: true,
    lookedBackTo: '2026-10-01T08:14:00Z',
  },
};
