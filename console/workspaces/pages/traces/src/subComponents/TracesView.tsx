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
import type { TraceOverview, TraceScoreSummary } from "@agent-management-platform/types";
import { TracesTable } from "./TracesTable";

export interface TracesViewProps {
  // Data props
  traces: TraceOverview[];
  isLoading?: boolean;
  sortOrder?: "asc" | "desc";
  selectedTrace: string | null;
  scoreMap?: Map<string, TraceScoreSummary>;
  isScoresLoading?: boolean;
  isLoadingOlder?: boolean;
  isLoadingNewer?: boolean;

  // Handlers
  onTraceSelect: (traceId: string) => void;
  onLoadOlder?: () => void;
  onLoadNewer?: () => void;
}

export const TracesView: React.FC<TracesViewProps> = ({
  traces,
  isLoading = false,
  sortOrder = "desc",
  selectedTrace,
  scoreMap,
  isScoresLoading = false,
  isLoadingOlder = false,
  isLoadingNewer = false,
  onTraceSelect,
  onLoadOlder,
  onLoadNewer,
}) => {
  return (
    <TracesTable
      isLoading={isLoading}
      sortOrder={sortOrder}
      traces={traces}
      onTraceSelect={onTraceSelect}
      selectedTrace={selectedTrace}
      scoreMap={scoreMap}
      isScoresLoading={isScoresLoading}
      isLoadingOlder={isLoadingOlder}
      isLoadingNewer={isLoadingNewer}
      onLoadOlder={onLoadOlder}
      onLoadNewer={onLoadNewer}
    />
  );
};
