import { 
  TableHead, 
  TableRow, 
  TableCell, 
  TableSortLabel 
} from '@wso2/oxygen-ui';
import { 
  Clipboard as TaskOutlined, 
} from '@wso2/oxygen-ui-icons-react';
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
  return (
    <TableHead>
      <TableRow>
        {columns.map((column) => (
          <TableCell
            key={String(column.id)}
            align={column.align || 'left'}
            sx={{ 
              width: column.width,
            }}
          >
            {column.sortable !== false ? (
              <TableSortLabel
                active={sortBy === column.id}
                direction={sortBy === column.id ? sortDirection : 'asc'}
                onClick={() => onSort(column.id)}
              >
                {column.label}
              </TableSortLabel>
            ) : (
              column.label
            )}
          </TableCell>
        ))}
        {hasActions && (
          <TableCell align="right">
            <TaskOutlined />
          </TableCell>
        )}
      </TableRow>
    </TableHead>
  );
};
