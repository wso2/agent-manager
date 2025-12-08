import { TableBody as MuiTableBody } from '@wso2/oxygen-ui';
import { TableColumn } from '../DataListingTable';
import { TableRow } from './TableRow';
import { ActionItem } from './ActionMenu';

export interface TableBodyProps<T = any> {
  data: T[];
  columns: TableColumn<T>[];
  actions?: ActionItem[];
  onRowAction?: (action: string, row: T) => void;
  onRowMouseEnter?: (row: T) => void;
  onRowMouseLeave?: (row: T) => void;
  onRowFocusIn?: (row: T) => void;
  onRowFocusOut?: (row: T) => void;
  onRowClick?: (row: T) => void;
}

export const TableBody = <T extends Record<string, any>>({
  data,
  columns,
  actions = [],
  onRowAction,
  onRowMouseEnter,
  onRowMouseLeave,
  onRowFocusIn,
  onRowFocusOut,
  onRowClick,
}: TableBodyProps<T>) => {
  return (
    <MuiTableBody>
      {data.map((row, index) => (
        <TableRow
          key={row.id || index}
          row={row}
          columns={columns}
          actions={actions}
          onRowAction={onRowAction}
          rowIndex={index}
          onRowMouseEnter={onRowMouseEnter}
          onRowMouseLeave={onRowMouseLeave}
          onRowClick={onRowClick}
          onRowFocusIn={onRowFocusIn}
          onRowFocusOut={onRowFocusOut}
        />
      ))}
    </MuiTableBody>
  );
};
