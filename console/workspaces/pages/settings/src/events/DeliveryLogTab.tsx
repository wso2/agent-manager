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

import { useEffect, useState } from "react";
import {
  Box,
  Button,
  Chip,
  Divider,
  IconButton,
  ListingTable,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { History, RefreshCw } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  TextInput,
} from "@agent-management-platform/views";
import { ResourceListShell } from "@agent-management-platform/shared-component";
import { useListEventDeliveries } from "@agent-management-platform/api-client";
import type {
  EventDeliveryResponse,
  EventDeliveryStatus,
  EventTargetParams,
} from "@agent-management-platform/types";
import { DeliveryStatusChip, DestinationName, timeAgo } from "./eventsUi";

const PAGE_SIZE = 25;

function where(d: EventDeliveryResponse): string {
  const parts = [d.projectName, d.agentName].filter(Boolean).join(" / ");
  const base = parts || "Organization";
  return d.environment ? `${base} · ${d.environment}` : base;
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Stack direction="row" spacing={2} alignItems="baseline">
      <Typography variant="body2" color="text.secondary" sx={{ width: 140, flexShrink: 0 }}>
        {label}
      </Typography>
      <Box sx={{ minWidth: 0, flex: 1 }}>{children}</Box>
    </Stack>
  );
}

function DeliveryDrawer({
  delivery,
  onClose,
}: {
  delivery?: EventDeliveryResponse;
  onClose: () => void;
}) {
  return (
    <DrawerWrapper
      open={!!delivery}
      onClose={onClose}
      minWidth="min(560px, 100vw)"
      maxWidth="min(620px, 100vw)"
    >
      <DrawerHeader icon={<History size={22} />} title="Delivery" onClose={onClose} />
      <DrawerContent>
        {delivery && (
          <Stack spacing={1.5}>
            <DetailRow label="Status">
              <DeliveryStatusChip status={delivery.status} attempts={delivery.attempts} />
            </DetailRow>
            <DetailRow label="Event">
              <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                {delivery.eventType}
              </Typography>
            </DetailRow>
            <DetailRow label="Event ID">
              <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
                {delivery.eventId}
              </Typography>
            </DetailRow>
            <DetailRow label="Where">
              <Typography variant="body2">{where(delivery)}</Typography>
            </DetailRow>
            <DetailRow label="Destination">
              {delivery.destination ? (
                <DestinationName
                  kind={delivery.destination.kind}
                  service={delivery.destination.service}
                  name={delivery.destination.name}
                />
              ) : (
                <Typography variant="body2" color="text.secondary">
                  Deleted
                </Typography>
              )}
            </DetailRow>
            <DetailRow label="Subscriptions">
              <Stack spacing={0.5}>
                {delivery.subscriptions.length ? (
                  delivery.subscriptions.map((s) => (
                    <Typography key={s.id} variant="body2">
                      {s.name}{" "}
                      <Typography component="span" variant="caption" color="text.secondary">
                        ({s.scope})
                      </Typography>
                    </Typography>
                  ))
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    Deleted since
                  </Typography>
                )}
              </Stack>
            </DetailRow>
            <Divider />
            <DetailRow label="Attempts">
              <Typography variant="body2">{delivery.attempts}</Typography>
            </DetailRow>
            <DetailRow label="Response">
              <Typography variant="body2">{delivery.responseCode ?? "None"}</Typography>
            </DetailRow>
            {delivery.lastError && (
              <DetailRow label="Last error">
                <Typography variant="body2" color="error.main" sx={{ wordBreak: "break-word" }}>
                  {delivery.lastError}
                </Typography>
              </DetailRow>
            )}
            <DetailRow label="Queued">
              <Typography variant="body2" title={delivery.createdAt}>
                {timeAgo(delivery.createdAt)}
              </Typography>
            </DetailRow>
            <DetailRow label="Last update">
              <Typography variant="body2" title={delivery.updatedAt}>
                {timeAgo(delivery.updatedAt)}
              </Typography>
            </DetailRow>
            {delivery.deliveredAt && (
              <DetailRow label="Delivered">
                <Typography variant="body2" title={delivery.deliveredAt}>
                  {timeAgo(delivery.deliveredAt)}
                </Typography>
              </DetailRow>
            )}
          </Stack>
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}

export interface DeliveryLogTabProps {
  target: EventTargetParams;
  /** Destinations to filter by, organization level only. */
  destinationOptions?: { id: string; name: string }[];
  destinationId?: string;
  subscriptionId?: string;
  subscriptionName?: string;
  onDestinationChange: (id: string | undefined) => void;
  onClearSubscription: () => void;
}

/** A level's delivery log: which events went where, and how it went. */
export function DeliveryLogTab({
  target,
  destinationOptions,
  destinationId,
  subscriptionId,
  subscriptionName,
  onDestinationChange,
  onClearSubscription,
}: DeliveryLogTabProps) {
  const [status, setStatus] = useState<EventDeliveryStatus | "">("");
  const [offset, setOffset] = useState(0);
  const [viewing, setViewing] = useState<EventDeliveryResponse | undefined>();
  const { data, isLoading, error, refetch, isFetching } = useListEventDeliveries(target, {
    destinationId,
    subscriptionId,
    status: status || undefined,
    limit: PAGE_SIZE,
    offset,
  });
  const deliveries = data?.deliveries ?? [];
  const total = data?.total ?? 0;
  // Rows can age out between refreshes; a page past the end would show
  // "No deliveries" with no way back, so step back to the last page.
  useEffect(() => {
    if (total > 0 && offset >= total) {
      setOffset(Math.floor((total - 1) / PAGE_SIZE) * PAGE_SIZE);
    }
  }, [offset, total]);

  const isFiltered = !!(status || destinationId || subscriptionId);
  const toolbar = (
    <Stack direction="row" justifyContent="flex-end">
      <Stack direction="row" alignItems="center" flexWrap="wrap" useFlexGap gap={1.5}>
        {subscriptionId && (
          <Chip
            label={`Subscription: ${subscriptionName ?? "selected"}`}
            onDelete={() => {
              setOffset(0);
              onClearSubscription();
            }}
          />
        )}
        {destinationOptions && (
          <Box sx={{ minWidth: 220 }}>
            <TextInput
              select
              size="small"
              aria-label="Filter by destination"
              value={destinationId ?? ""}
              onChange={(e) => {
                setOffset(0);
                onDestinationChange(e.target.value || undefined);
              }}
              slotProps={{
                select: {
                  displayEmpty: true,
                  renderValue: (v) =>
                    `Destination: ${v ? (destinationOptions.find((d) => d.id === v)?.name ?? "selected") : "All"}`,
                },
              }}
            >
              <MenuItem value="">All</MenuItem>
              {destinationOptions.map((d) => (
                <MenuItem key={d.id} value={d.id}>
                  {d.name}
                </MenuItem>
              ))}
            </TextInput>
          </Box>
        )}
        <Box sx={{ minWidth: 170 }}>
          <TextInput
            select
            size="small"
            aria-label="Filter by status"
            value={status}
            onChange={(e) => {
              setOffset(0);
              setStatus(e.target.value as EventDeliveryStatus | "");
            }}
            slotProps={{
              select: {
                displayEmpty: true,
                renderValue: (v) =>
                  `Status: ${v ? String(v).charAt(0).toUpperCase() + String(v).slice(1) : "All"}`,
              },
            }}
          >
            <MenuItem value="">All</MenuItem>
            <MenuItem value="delivered">Delivered</MenuItem>
            <MenuItem value="failed">Failed</MenuItem>
            <MenuItem value="pending">Pending or retrying</MenuItem>
            <MenuItem value="skipped">Skipped</MenuItem>
          </TextInput>
        </Box>
        <Tooltip title="Refresh">
          <span>
            <IconButton
              aria-label="Refresh delivery log"
              onClick={() => refetch()}
              disabled={isFetching}
            >
              <RefreshCw size={16} />
            </IconButton>
          </span>
        </Tooltip>
      </Stack>
    </Stack>
  );

  return (
    <>
      <ResourceListShell
        resourceName="event-deliveries"
        searchValue=""
        onSearchChange={() => undefined}
        toolbar={toolbar}
        error={error ? new Error("Couldn't load the delivery log. Reload the page to try again.") : undefined}
        isLoading={isLoading}
        // A filtered log keeps its filters on screen, so it is never the bare empty state.
        isEmpty={!isLoading && total === 0 && !isFiltered}
        isSearchEmpty={!isLoading && deliveries.length === 0 && isFiltered}
        emptyState={{
          illustration: <History size={64} />,
          title: "No deliveries yet",
          description: "Deliveries appear here as events are sent to destinations.",
        }}
        searchEmptyState={{
          illustration: <History size={64} />,
          title: "No deliveries match",
          description: "Nothing matches these filters in the retention window.",
        }}
      >
    <>
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Time</ListingTable.Cell>
            <ListingTable.Cell>Event</ListingTable.Cell>
            <ListingTable.Cell>Where</ListingTable.Cell>
            <ListingTable.Cell>Destination</ListingTable.Cell>
            <ListingTable.Cell>Subscriptions</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
            <ListingTable.Cell align="right">Response</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {deliveries.map((d) => (
            <ListingTable.Row
              key={d.id}
              hover
              sx={{ cursor: "pointer" }}
              onClick={() => setViewing(d)}
            >
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary" noWrap title={d.createdAt}>
                  {timeAgo(d.createdAt)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Chip
                  size="small"
                  label={d.eventType}
                  sx={{ fontFamily: "monospace", fontSize: 12 }}
                />
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" noWrap>
                  {where(d)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                {d.destination ? (
                  <DestinationName
                    kind={d.destination.kind}
                    service={d.destination.service}
                    name={d.destination.name}
                  />
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    Deleted
                  </Typography>
                )}
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography
                  variant="body2"
                  noWrap
                  title={d.subscriptions.map((s) => s.name).join(", ")}
                >
                  {d.subscriptions.length === 0
                    ? "—"
                    : d.subscriptions.length === 1
                      ? d.subscriptions[0].name
                      : `${d.subscriptions[0].name} +${d.subscriptions.length - 1}`}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <DeliveryStatusChip status={d.status} attempts={d.attempts} />
              </ListingTable.Cell>
              <ListingTable.Cell align="right">
                <Typography variant="body2" color="text.secondary">
                  {d.responseCode ?? "—"}
                </Typography>
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
      <Stack
        direction="row"
        justifyContent="flex-end"
        alignItems="center"
        spacing={1.5}
        sx={{ px: 2, py: 1 }}
      >
        <Typography variant="caption" color="text.secondary">
          {`${offset + 1}–${Math.min(offset + PAGE_SIZE, total)} of ${total}`}
        </Typography>
        <Button
          size="small"
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
        >
          Newer
        </Button>
        <Button
          size="small"
          disabled={offset + PAGE_SIZE >= total}
          onClick={() => setOffset(offset + PAGE_SIZE)}
        >
          Older
        </Button>
      </Stack>
    </>
      </ResourceListShell>
      <DeliveryDrawer delivery={viewing} onClose={() => setViewing(undefined)} />
    </>
  );
}

export default DeliveryLogTab;
