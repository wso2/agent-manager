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

import {
  Chip,
  IconButton,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { History, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { DrawerContent, DrawerHeader, DrawerWrapper } from "@agent-management-platform/views";
import { useWebhookDeliveries } from "@agent-management-platform/api-client";
import type {
  WebhookDeliveryStatus,
  WebhookResponse,
  WebhookTargetParams,
} from "@agent-management-platform/types";

const STATUS_COLOR: Record<WebhookDeliveryStatus, "success" | "warning" | "error"> = {
  delivered: "success",
  pending: "warning",
  failed: "error",
};

export interface WebhookDeliveriesDrawerProps {
  target: WebhookTargetParams;
  webhook?: WebhookResponse;
  onClose: () => void;
}

/** Recent deliveries of one webhook, newest first. */
export function WebhookDeliveriesDrawer({
  target,
  webhook,
  onClose,
}: WebhookDeliveriesDrawerProps) {
  const { data, isLoading, refetch } = useWebhookDeliveries(target, webhook?.id);
  const deliveries = data?.deliveries ?? [];

  return (
    <DrawerWrapper open={!!webhook} onClose={onClose} maxWidth={760}>
      <DrawerHeader
        icon={<History size={24} />}
        title={`Deliveries: ${webhook?.name ?? ""}`}
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <Typography variant="body2" color="text.secondary">
              Failed attempts are retried after 10s, 1m, 5m, 30m and then every 2h.
            </Typography>
            <IconButton size="small" aria-label="Refresh deliveries" onClick={() => refetch()}>
              <RefreshCcw size={16} />
            </IconButton>
          </Stack>
          {isLoading ? (
            <Skeleton variant="rounded" height={160} />
          ) : deliveries.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No events have been sent to this webhook yet.
            </Typography>
          ) : (
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Event</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Attempts</TableCell>
                  <TableCell>Response</TableCell>
                  <TableCell>When</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {deliveries.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell sx={{ fontFamily: "monospace" }}>{d.eventType}</TableCell>
                    <TableCell>
                      <Chip label={d.status} size="small" color={STATUS_COLOR[d.status]} />
                    </TableCell>
                    <TableCell>{d.attempts}</TableCell>
                    <TableCell title={d.lastError}>
                      {d.responseCode ?? (d.lastError ? "error" : "—")}
                    </TableCell>
                    <TableCell>
                      {formatDistanceToNow(new Date(d.updatedAt), { addSuffix: true })}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default WebhookDeliveriesDrawer;
