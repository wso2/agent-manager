import { TableRow as OxygenTableRow, TableCell } from '@wso2/oxygen-ui';
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
  const getNestedValue = (obj: any, path: string | number | symbol) => {
    return String(path).split('.').reduce((current, key) => current?.[key], obj);
  };

  return (
    <OxygenTableRow
      hover = {!!onRowClick}
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
        cursor: onRowClick ? 'pointer' : 'default',
      }}
    >
      {columns.map((column) => (
        <TableCell
          key={String(column.id)}
          align={column.align || 'left'}
        >
          {column.render ? (
            column.render(getNestedValue(row, column.id), row)
          ) : (
            getNestedValue(row, column.id)
          )}
        </TableCell>
      ))}
      {actions.length > 0 && (
        <TableCell align="right">
          <ActionMenu
            row={row}
            actions={actions}
            onActionClick={onRowAction || (() => { })}
          />
        </TableCell>
      )}
    </OxygenTableRow>
  );
};
