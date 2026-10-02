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
  sortOrder?: "asc" | "desc";
  selectedTrace: string | null;
  isLoadingOlder?: boolean;
  isLoadingNewer?: boolean;
  hasOlder?: boolean;
  hasActiveFilters?: boolean;
  // The server stopped early: the examine cap with hasOlder, the cursor depth cap without.
  truncated?: boolean;
  lookedBackTo?: string;
  visibleColumns?: TraceColumn[];

  // Handlers
  onTraceSelect: (traceId: string) => void;
  onLoadOlder?: () => void;
  onLoadNewer?: () => void;
  onConversationSelect?: (conversationId: string) => void;
}

export const TracesView: React.FC<TracesViewProps> = ({
  traces,
  isLoading = false,
  sortOrder = "desc",
  selectedTrace,
  isLoadingOlder = false,
  isLoadingNewer = false,
  hasOlder = false,
  hasActiveFilters = false,
  truncated = false,
  lookedBackTo,
  visibleColumns,
  onTraceSelect,
  onLoadOlder,
  onLoadNewer,
  onConversationSelect,
}) => {
  return (
    <>
      {truncated && !isLoading && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {hasOlder
            ? "Showing matches from the first 500 traces examined. Narrow the time range to see more."
            : "The list stops here: older traces are more than 5,000 traces into this time range. Narrow the time range to see more."}
        </Alert>
      )}
      <TracesTable
        isLoading={isLoading}
        sortOrder={sortOrder}
        traces={traces}
        onTraceSelect={onTraceSelect}
        selectedTrace={selectedTrace}
        isLoadingOlder={isLoadingOlder}
        isLoadingNewer={isLoadingNewer}
        hasOlder={hasOlder}
        hasActiveFilters={hasActiveFilters}
        visibleColumns={visibleColumns}
        lookedBackTo={lookedBackTo}
        onLoadOlder={onLoadOlder}
        onLoadNewer={onLoadNewer}
        onConversationSelect={onConversationSelect}
      />
    </>
  );
};
