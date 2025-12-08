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
