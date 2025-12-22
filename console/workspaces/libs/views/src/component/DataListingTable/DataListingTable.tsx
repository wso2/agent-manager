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

import React, { useState, useMemo } from 'react';
import {
  Table,
  TableContainer,
  Paper,
  TablePagination,
} from '@wso2/oxygen-ui';
import { TableHeader } from './subcomponents/TableHeader';
import { TableBody } from './subcomponents/TableBody';
import { LoadingState } from './subcomponents/LoadingState';
import { ActionItem } from './subcomponents/ActionMenu';
import {
  BoxIcon ,
} from '@wso2/oxygen-ui-icons-react';
import { NoDataFound } from '../NoDataFound';

export interface TableColumn<T = any> {
  id: keyof T | string;
  label: string;
  sortable?: boolean;
  render?: (value: T[keyof T], row: T) => React.ReactNode;
  width?: string | number;
  align?: 'left' | 'center' | 'right';
}


export interface MetricsData {
  metricsValue: string | number;
  metricsColor: 'success' | 'warning' | 'error';
}

export interface SortModel<T = any> {
  field: keyof T | string;
  sort: 'asc' | 'desc';
}

export interface InitialState<T = any> {
  sorting?: {
    sortModel?: SortModel<T>[];
  };
}

export interface DataListingTableProps<T = any> {
  data: T[];
  columns: TableColumn<T>[];
  loading?: boolean;
  onRowAction?: (action: string, row: T) => void;
  actions?: ActionItem[];
  emptyStateTitle?: string;
  emptyStateDescription?: string;
  emptyStateActionLabel?: string;
  onEmptyStateAction?: () => void;
  // Pagination props
  pagination?: boolean;
  pageSize?: number;
  maxRows?: number;
  onPageChange?: (page: number, rowsPerPage: number) => void;
  // Sorting props
  initialState?: InitialState<T>;
  defaultSortBy?: keyof T | string;
  defaultSortDirection?: 'asc' | 'desc';
  // Row mouse events
  onRowMouseEnter?: (row: T) => void;
  onRowMouseLeave?: (row: T) => void;
  // Row focus events for accessibility
  onRowFocusIn?: (row: T) => void;
  onRowFocusOut?: (row: T) => void;
  onRowClick?: (row: T) => void;
}

type SortDirection = 'asc' | 'desc';

export const DataListingTable = <T extends Record<string, any>>({
  data,
  columns,
  loading = false,
  onRowAction,
  actions = [],
  pagination = false,
  pageSize = 10,
  maxRows,
  onPageChange,
  initialState,
  defaultSortBy,
  defaultSortDirection = 'asc',
  onRowMouseEnter,
  onRowMouseLeave,
  onRowFocusIn,
  onRowFocusOut,
  onRowClick,
  emptyStateTitle = 'No data found',
  emptyStateDescription = 'No data found',
}: DataListingTableProps<T>) => {
  // Determine initial sort values from initialState or fallback to defaultSort props
  const getInitialSortBy = () => {
    if (
      initialState?.sorting?.sortModel &&
      initialState.sorting.sortModel.length > 0
    ) {
      return initialState.sorting.sortModel[0].field;
    }
    return defaultSortBy || '';
  };

  const getInitialSortDirection = (): SortDirection => {
    if (
      initialState?.sorting?.sortModel &&
      initialState.sorting.sortModel.length > 0
    ) {
      return initialState.sorting.sortModel[0].sort;
    }
    return defaultSortDirection;
  };

  const [sortBy, setSortBy] = useState<keyof T | string>(getInitialSortBy());
  const [sortDirection, setSortDirection] = useState<SortDirection>(
    getInitialSortDirection()
  );
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(pageSize);

  const getNestedValue = (obj: any, path: string | number | symbol) => {
    return String(path)
      .split('.')
      .reduce((current, key) => current?.[key], obj);
  };

  const handleSort = (columnId: keyof T | string) => {
    if (sortBy === columnId) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(columnId);
      setSortDirection('asc');
    }
  };

  const handleChangePage = (_event: unknown, newPage: number) => {
    setPage(newPage);
    if (onPageChange) {
      onPageChange(newPage, rowsPerPage);
    }
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    const newRowsPerPage = parseInt(event.target.value, 10);
    setRowsPerPage(newRowsPerPage);
    setPage(0);
    if (onPageChange) {
      onPageChange(0, newRowsPerPage);
    }
  };

  const sortedData = useMemo(() => {
    if (!sortBy) return data;

    return [...data].sort((a, b) => {
      const aValue = getNestedValue(a, sortBy);
      const bValue = getNestedValue(b, sortBy);

      if (aValue === bValue) return 0;

      const comparison = aValue < bValue ? -1 : 1;
      return sortDirection === 'asc' ? comparison : -comparison;
    });
  }, [data, sortBy, sortDirection]);

  // Calculate pagination data
  const totalRows = maxRows || data.length;
  const paginatedData = useMemo(() => {
    if (!pagination) return sortedData;

    const startIndex = page * rowsPerPage;
    const endIndex = startIndex + rowsPerPage;
    return sortedData.slice(startIndex, endIndex);
  }, [sortedData, page, rowsPerPage, pagination]);

  if (loading) {
    return <LoadingState />;
  }

  if (data.length === 0) {
    return (
      <NoDataFound
        message={emptyStateTitle}
        subtitle={emptyStateDescription}
        iconElement={BoxIcon}
      />
    );
  }

  return (
    <Paper
      sx={{
        width: '100%',
        "& .MuiTableContainer-root": {
          backgroundColor: 'background.default',
        },
      }}
    >
      <TableContainer>
        <Table>
          <TableHeader
            columns={columns}
            sortBy={sortBy}
            sortDirection={sortDirection}
            onSort={handleSort}
            hasActions={actions.length > 0}
          />
          <TableBody
            data={paginatedData}
            columns={columns}
            actions={actions}
            onRowAction={onRowAction}
            onRowMouseEnter={onRowMouseEnter}
            onRowMouseLeave={onRowMouseLeave}
            onRowFocusIn={onRowFocusIn}
            onRowFocusOut={onRowFocusOut}
            onRowClick={onRowClick}
          />
        </Table>
      </TableContainer>
      {pagination && totalRows > 5 && (
        <TablePagination
          rowsPerPageOptions={[5, 10, 25, 50]}
          component="div"
          count={totalRows}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      )}
    </Paper>
  );
};
