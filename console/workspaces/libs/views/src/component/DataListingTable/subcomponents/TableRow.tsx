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

import { TableRow as MuiTableRow, TableCell, useTheme } from '@mui/material';
import { TableColumn } from '../DataListingTable';
import { ActionMenu, ActionItem } from './ActionMenu';

export interface TableRowProps<T = any> {
  row: T;
  columns: TableColumn<T>[];
  actions?: ActionItem[];
  onRowAction?: (action: string, row: T) => void;
  rowIndex: number;
  onRowMouseEnter?: (row: T) => void;
  onRowMouseLeave?: (row: T) => void;
  onRowFocusIn?: (row: T) => void;
  onRowFocusOut?: (row: T) => void;
  onRowClick?: (row: T) => void;
}

export const TableRow = <T extends Record<string, any>>({
  row,
  columns,
  actions = [],
  onRowAction,
  onRowMouseEnter,
  onRowMouseLeave,
  onRowFocusIn,
  onRowFocusOut,
  onRowClick,
}: TableRowProps<T>) => {
  const theme = useTheme();

  const getNestedValue = (obj: any, path: string | number | symbol) => {
    return String(path).split('.').reduce((current, key) => current?.[key], obj);
  };

  return (
    <MuiTableRow
      hover
      onClick={onRowClick ? () => onRowClick(row) : undefined}
      onMouseEnter={onRowMouseEnter ? () => onRowMouseEnter(row) : undefined}
      onMouseLeave={onRowMouseLeave ? () => onRowMouseLeave(row) : undefined}
      onFocus={onRowFocusIn ? (e) => {
        // Only trigger if focus is coming from outside the row
        if (!e.currentTarget.contains(e.relatedTarget as Node)) {
          onRowFocusIn(row);
        }
      } : undefined}
      onBlur={onRowFocusOut ? (e) => {
        // Only trigger if focus is leaving the row
        if (!e.currentTarget.contains(e.relatedTarget as Node)) {
          onRowFocusOut(row);
        }
      } : undefined}
      sx={{
        cursor: onRowAction ? 'pointer' : 'default',
        backgroundColor: theme.palette.background.paper,
        boxShadow: theme.shadows[1],
        transition: theme.transitions.create(['box-shadow', 'background-color'], {
          duration: theme.transitions.duration.short,
        }),
        '&:hover': {
          // boxShadow: theme.shadows[2],
          backgroundColor: theme.palette.background.paper,
          // backgroundColor: 'red'
        },
        '& td': {
          borderTop: `1px solid ${theme.palette.divider}`,
          borderBottom: `1px solid ${theme.palette.divider}`,
        },
        '& td:first-of-type': {
          borderLeft: `1px solid ${theme.palette.divider}`,
          borderTopLeftRadius: theme.spacing(1),
          borderBottomLeftRadius: theme.spacing(1),
        },
        '& td:last-of-type': {
          borderRight: `1px solid ${theme.palette.divider}`,
          borderTopRightRadius: theme.spacing(1),
          borderBottomRightRadius: theme.spacing(1),
        },
      }}
    >
      {columns.map((column) => (
        <TableCell
          key={String(column.id)}
          align={column.align || 'left'}
          sx={{
            padding: theme.spacing(1, 2),
            fontSize: theme.typography.body2.fontSize,
            color: theme.palette.text.primary,
            borderBottom: 'none',
          }}
        >
          {column.render ? (
            column.render(getNestedValue(row, column.id), row)
          ) : (
            getNestedValue(row, column.id)
          )}
        </TableCell>
      ))}
      {actions.length > 0 && (
        <TableCell
          align="right"
          sx={{
            padding: theme.spacing(1.5, 2),
            fontSize: theme.typography.body2.fontSize,
            color: theme.palette.text.primary,
            borderBottom: 'none',
          }}
        >
          <ActionMenu
            row={row}
            actions={actions}
            onActionClick={onRowAction || (() => { })}
          />
        </TableCell>
      )}
    </MuiTableRow>
  );
};
