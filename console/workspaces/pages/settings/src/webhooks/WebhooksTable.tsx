/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useMemo, useState, type ReactNode } from "react";
import {
  Alert,
  Chip,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Edit,
  History,
  KeyRound,
  Send,
  Trash,
  Webhook,
} from "@wso2/oxygen-ui-icons-react";
import type { WebhookResponse } from "@agent-management-platform/types";

export interface WebhooksTableProps {
  webhooks: WebhookResponse[];
  isLoading: boolean;
  error?: unknown;
  canManage: boolean;
  /** Shows the environments column (agent webhooks). */
  showEnvironments: boolean;
  envLabel: (name: string) => string;
  testingId?: string;
  /** Shown at the right of the search bar, like Create Role on the roles page. */
  toolbarActions?: ReactNode;
  onEdit: (w: WebhookResponse) => void;
  onTest: (w: WebhookResponse) => void;
  onDeliveries: (w: WebhookResponse) => void;
  onRotate: (w: WebhookResponse) => void;
  onDelete: (w: WebhookResponse) => void;
}

function ChipList({ values, max = 2 }: { values: string[]; max?: number }) {
  if (values.length === 0) {
    return (
      <Typography variant="caption" color="text.secondary">
        None
      </Typography>
    );
  }
  return (
    <Stack direction="row" flexWrap="nowrap" alignItems="center" gap={1} sx={{ minWidth: 0 }}>
      {values.slice(0, max).map((v) => (
        <Chip key={v} size="small" label={v} sx={{ minWidth: 0, maxWidth: 180 }} />
      ))}
      {values.length > max && (
        <Tooltip title={values.join(", ")}>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ whiteSpace: "nowrap", flexShrink: 0 }}
          >
            {`+${values.length - max} more`}
          </Typography>
        </Tooltip>
      )}
    </Stack>
  );
}

/** The webhooks list, styled like the monitors table. */
export function WebhooksTable({
  webhooks,
  isLoading,
  error,
  canManage,
  showEnvironments,
  envLabel,
  testingId,
  toolbarActions,
  onEdit,
  onTest,
  onDeliveries,
  onRotate,
  onDelete,
}: WebhooksTableProps) {
  const [searchValue, setSearchValue] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const filtered = useMemo(() => {
    const q = searchValue.trim().toLowerCase();
    if (!q) return webhooks;
    return webhooks.filter(
      (w) => w.name.toLowerCase().includes(q) || w.url.toLowerCase().includes(q),
    );
  }, [webhooks, searchValue]);

  const toolbar = (
    <ListingTable.Toolbar
      showSearch
      searchValue={searchValue}
      onSearchChange={(v: string) => {
        setSearchValue(v);
        setPage(0);
      }}
      searchPlaceholder="Search webhooks..."
      actions={toolbarActions}
    />
  );

  if (error) {
    return (
      <ListingTable.Container>
        {toolbar}
        <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ alignSelf: "stretch" }}>
          Failed to load webhooks. Please try again.
        </Alert>
      </ListingTable.Container>
    );
  }

  if (isLoading) {
    return (
      <ListingTable.Container>
        {toolbar}
        <Stack spacing={1} m={2}>
          <Skeleton variant="rounded" height={60} />
          <Skeleton variant="rounded" height={60} />
          <Skeleton variant="rounded" height={60} />
        </Stack>
      </ListingTable.Container>
    );
  }

  if (!webhooks.length || !filtered.length) {
    return (
      <ListingTable.Container>
        {toolbar}
        <ListingTable.EmptyState
          illustration={<Webhook size={64} />}
          title={webhooks.length ? "No webhooks match your search" : "No webhooks yet"}
          description={
            webhooks.length
              ? "Try adjusting your search keywords."
              : "Add a webhook to receive signed HTTP requests when the events you choose happen."
          }
        />
      </ListingTable.Container>
    );
  }

  const rows = filtered.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

  return (
    <ListingTable.Container>
      {toolbar}
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell align="center">Status</ListingTable.Cell>
            {showEnvironments && <ListingTable.Cell>Environments</ListingTable.Cell>}
            <ListingTable.Cell>Events</ListingTable.Cell>
            <ListingTable.Cell>Actions</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {rows.map((w) => (
            <ListingTable.Row
              key={w.id}
              hover
              sx={{ cursor: canManage ? "pointer" : "default" }}
              onClick={() => canManage && onEdit(w)}
            >
              <ListingTable.Cell>
                <Stack spacing={0.5} sx={{ minWidth: 0, maxWidth: 360 }}>
                  <Typography variant="body2">{w.name}</Typography>
                  <Typography variant="caption" color="text.secondary" noWrap title={w.url}>
                    {w.url}
                  </Typography>
                </Stack>
              </ListingTable.Cell>
              <ListingTable.Cell align="center">
                <Chip
                  size="small"
                  variant="outlined"
                  label={w.enabled ? "Enabled" : "Disabled"}
                  color={w.enabled ? "success" : "default"}
                />
              </ListingTable.Cell>
              {showEnvironments && (
                <ListingTable.Cell>
                  <ChipList values={w.environments.map(envLabel)} />
                </ListingTable.Cell>
              )}
              <ListingTable.Cell>
                <ChipList values={w.eventTypes} />
              </ListingTable.Cell>
              <ListingTable.Cell onClick={(e) => e.stopPropagation()}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Tooltip title="Send test event">
                    <span>
                      <IconButton
                        aria-label={`Send test event to ${w.name}`}
                        disabled={!canManage || testingId === w.id}
                        onClick={() => onTest(w)}
                      >
                        <Send size={16} />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="Deliveries">
                    <IconButton aria-label={`Deliveries of ${w.name}`} onClick={() => onDeliveries(w)}>
                      <History size={16} />
                    </IconButton>
                  </Tooltip>
                  <IconButton
                    aria-label={`Edit webhook ${w.name}`}
                    disabled={!canManage}
                    onClick={() => onEdit(w)}
                  >
                    <Edit size={16} />
                  </IconButton>
                  <Tooltip title="Rotate signing secret">
                    <span>
                      <IconButton
                        aria-label={`Rotate signing secret of ${w.name}`}
                        disabled={!canManage}
                        onClick={() => onRotate(w)}
                      >
                        <KeyRound size={16} />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="Delete Webhook">
                    <span>
                      <IconButton
                        color="error"
                        aria-label={`Delete webhook ${w.name}`}
                        disabled={!canManage}
                        onClick={() => onDelete(w)}
                      >
                        <Trash size={16} />
                      </IconButton>
                    </span>
                  </Tooltip>
                </Stack>
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
      {filtered.length > rowsPerPage && (
        <TablePagination
          component="div"
          count={filtered.length}
          page={page}
          rowsPerPage={rowsPerPage}
          onPageChange={(_, p) => setPage(p)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
          rowsPerPageOptions={[5, 10, 25]}
        />
      )}
    </ListingTable.Container>
  );
}

export default WebhooksTable;
