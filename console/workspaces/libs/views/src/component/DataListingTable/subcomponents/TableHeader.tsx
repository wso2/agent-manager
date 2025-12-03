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

import { TableHead, TableRow, TableCell, TableSortLabel, useTheme } from '@mui/material';
import { ArrowUpward, ArrowDownward, TaskOutlined } from '@mui/icons-material';
import { TableColumn } from '../DataListingTable';

export interface TableHeaderProps<T = any> {
  columns: TableColumn<T>[];
  sortBy: keyof T | string;
  sortDirection: 'asc' | 'desc';
  onSort: (columnId: keyof T | string) => void;
  hasActions?: boolean;
}

export const TableHeader = <T extends Record<string, any>>({
  columns,
  sortBy,
  sortDirection,
  onSort,
  hasActions = false,
}: TableHeaderProps<T>) => {
  const theme = useTheme();

  const renderSortIcon = (columnId: keyof T | string) => {
    if (sortBy !== columnId) return null;
    return sortDirection === 'asc' ? <ArrowUpward fontSize="small" /> : <ArrowDownward fontSize="small" />;
  };

  return (
    <TableHead>
      <TableRow>
        {columns.map((column) => (
          <TableCell
            key={String(column.id)}
            align={column.align || 'left'}
            sx={{ 
              width: column.width,
              // fontWeight: 600,
              backgroundColor: 'transparent',
              borderBottom: 'none',
              padding: theme.spacing(1.5, 2),
              // fontSize: '0.875rem',
              fontSize: theme.typography.caption.fontSize,
              color: theme.palette.text.secondary,
            }}
          >
            {column.sortable !== false ? (
              <TableSortLabel
                active={sortBy === column.id}
                direction={sortBy === column.id ? sortDirection : 'asc'}
                onClick={() => onSort(column.id)}
                IconComponent={() => renderSortIcon(column.id)}
                sx={{
                  '&.MuiTableSortLabel-root': {
                    color: theme.palette.text.secondary,
                    '&:hover': {
                      color: theme.palette.text.primary,
                    },
                    '&.Mui-active': {
                      color: theme.palette.text.primary,
                    },
                  },
                }}
              >
                {column.label}
              </TableSortLabel>
            ) : (
              column.label
            )}
          </TableCell>
        ))}
        {hasActions && (
          <TableCell align="right" sx={{ 
            // fontWeight: 600,
            backgroundColor: 'transparent',
            borderBottom: 'none',
            padding: theme.spacing(1.5, 2),
            // fontSize: '0.875rem',
            color: theme.palette.text.secondary,
          }}>
            <TaskOutlined />
          </TableCell>
        )}
      </TableRow>
    </TableHead>
  );
};
