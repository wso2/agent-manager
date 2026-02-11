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
import { TracesTable } from "./TracesTable";

export interface TracesViewProps {
  // Data props
  traces: TraceOverview[];
  count: number;
  page: number;
  rowsPerPage: number;
  isLoading?: boolean;
  selectedTrace: string | null;

  // Handlers
  onTraceSelect: (traceId: string) => void;
  onPageChange: (page: number) => void;
  onRowsPerPageChange: (rowsPerPage: number) => void;
}

export const TracesView: React.FC<TracesViewProps> = ({
  traces,
  count,
  page,
  rowsPerPage,
  isLoading = false,
  selectedTrace,
  onTraceSelect,
  onPageChange,
  onRowsPerPageChange,
}) => {
  return (
    <TracesTable
      isLoading={isLoading}
      traces={traces}
      onTraceSelect={onTraceSelect}
      count={count}
      page={page}
      rowsPerPage={rowsPerPage}
      onPageChange={onPageChange}
      onRowsPerPageChange={onRowsPerPageChange}
      selectedTrace={selectedTrace}
    />
  );
};
