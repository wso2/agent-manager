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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import type { TraceOverview } from "@agent-management-platform/types";
import { Alert } from "@wso2/oxygen-ui";
import type { TraceColumn } from "../traceColumns";
import { TracesTable } from "./TracesTable";

export interface TracesViewProps {
  // Data props
  traces: TraceOverview[];
  isLoading?: boolean;
  selectedTrace: string | null;
  isLoadingMore?: boolean;
  hasMore?: boolean;
  hasActiveFilters?: boolean;
  hasScoreFilter?: boolean;
  // The evaluator the Score column shows while an evaluator filter is on.
  scoreEvaluator?: string;
  // The server stopped early: the examine cap with hasMore, the cursor depth cap without.
  truncated?: boolean;
  lookedBackTo?: string;
  loadError?: Error | null;
  visibleColumns?: TraceColumn[];

  // Handlers
  onTraceSelect: (traceId: string) => void;
  onLoadMore?: () => Promise<boolean | undefined>;
  onConversationSelect?: (conversationId: string) => void;
}

export const TracesView: React.FC<TracesViewProps> = ({
  traces,
  isLoading = false,
  selectedTrace,
  isLoadingMore = false,
  hasMore = false,
  hasActiveFilters = false,
  hasScoreFilter = false,
  scoreEvaluator,
  truncated = false,
  lookedBackTo,
  loadError,
  visibleColumns,
  onTraceSelect,
  onLoadMore,
  onConversationSelect,
}) => {
  return (
    <>
      {truncated && !isLoading && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {hasMore
            ? "Showing matches from the traces searched so far. Narrow the time range to see more."
            : "The list stops here: the remaining traces are more than 1,000 traces into this time range. Narrow the time range to see more."}
        </Alert>
      )}
      <TracesTable
        isLoading={isLoading}
        traces={traces}
        onTraceSelect={onTraceSelect}
        selectedTrace={selectedTrace}
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        hasActiveFilters={hasActiveFilters}
        hasScoreFilter={hasScoreFilter}
        scoreEvaluator={scoreEvaluator}
        visibleColumns={visibleColumns}
        lookedBackTo={lookedBackTo}
        loadError={loadError}
        onLoadMore={onLoadMore}
        onConversationSelect={onConversationSelect}
      />
    </>
  );
};
