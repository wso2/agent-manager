import React, { useState, useMemo } from 'react';
import { 
  Table, 
  TableContainer, 
  Paper, 
  Box, 
  Typography, 
  TablePagination, 
  Chip, 
  CircularProgress 
} from '@wso2/oxygen-ui';
import { TableHeader } from './subcomponents/TableHeader';
import { TableBody } from './subcomponents/TableBody';
import { LoadingState } from './subcomponents/LoadingState';
import { EmptyState } from './subcomponents/EmptyState';
import { ActionItem } from './subcomponents/ActionMenu';
import { 
  CheckCircle, 
  Circle as CircleOutlined, 
  XCircle as ErrorOutline 
} from '@wso2/oxygen-ui-icons-react';

export interface TableColumn<T = any> {
  id: keyof T | string;
  label: string;
  sortable?: boolean;
  render?: (value: T[keyof T], row: T) => React.ReactNode;
  width?: string | number;
  align?: 'left' | 'center' | 'right';
}

export interface StatusConfig {
  color: 'success' | 'warning' | 'error' | 'default';
  label: string;
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
}: DataListingTableProps<T>) => {
  // Determine initial sort values from initialState or fallback to defaultSort props
  const getInitialSortBy = () => {
    if (initialState?.sorting?.sortModel && initialState.sorting.sortModel.length > 0) {
      return initialState.sorting.sortModel[0].field;
    }
    return defaultSortBy || '';
  };

  const getInitialSortDirection = (): SortDirection => {
    if (initialState?.sorting?.sortModel && initialState.sorting.sortModel.length > 0) {
      return initialState.sorting.sortModel[0].sort;
    }
    return defaultSortDirection;
  };

  const [sortBy, setSortBy] = useState<keyof T | string>(getInitialSortBy());
  const [sortDirection, setSortDirection] = useState<SortDirection>(getInitialSortDirection());
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(pageSize);

  const getNestedValue = (obj: any, path: string | number | symbol) => {
    return String(path).split('.').reduce((current, key) => current?.[key], obj);
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

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
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
      <EmptyState

      />
    );
  }

  return (
    <Paper 
      sx={{ 
        width: '100%',
        boxShadow: 'none',
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

const getStatusIcon = (status: StatusConfig) => {
  switch (status.color) {
    case 'success':
      return <CheckCircle size={16} />;
    case 'warning':
      return <CircularProgress size={14} color="warning" />;
    case 'error':
      return <ErrorOutline size={16} />;
    default:
      return <CircleOutlined size={16} />;
  }
};
// Generic helper functions for common use cases
export const renderStatusChip = (status: StatusConfig, theme?: any) => (
  <Box display="flex" alignItems="center" gap={theme?.spacing(1) || 1}>
    <Chip variant="outlined"
      icon={getStatusIcon(status)}
      label={status.label}
      color={status.color}
      size="small"
    />
  </Box>
);

export const renderMetrics = (metrics: MetricsData, theme?: any) => (
  <Box>
    <Box display="flex" alignItems="center" gap={theme?.spacing(1) || 1} marginBottom={theme?.spacing(0.5) || 0.5}>
      <Box
        width={6}
        height={6}
        borderRadius="50%"
        bgcolor={
          metrics.metricsColor === 'success' ? (theme?.palette.success.main) :
            metrics.metricsColor === 'warning' ? (theme?.palette.warning.main) :
              (theme?.palette.error.main)
        }
      />
      <Typography
        variant="body2"
        sx={{
          color: theme?.palette.text.secondary,
          fontSize: theme?.typography.body2.fontSize,
        }}
      >
        {metrics.metricsValue}
      </Typography>
    </Box>
  </Box>
);
