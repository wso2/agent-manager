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

import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  ListingTable,
  Menu,
  MenuItem,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { MoreHorizontal, Plus, Send, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";
import {
  useDeleteEventDestination,
  useDestinationKinds,
  useEventDestination,
  useListEventDestinations,
  useRotateEventDestinationSecret,
  useTestEventDestination,
} from "@agent-management-platform/api-client";
import {
  isListEmpty,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import type {
  EventDestinationResponse,
  EventDestinationTestResponse,
  EventScope,
} from "@agent-management-platform/types";
import {
  availabilityLabel,
  DestinationIcon,
  HealthChip,
  lookOf,
  SecretReveal,
  timeAgo,
} from "./eventsUi";

/** What a destination row says under its name. */
function subtitleOf(d: EventDestinationResponse, serviceLabel: string): string {
  if (d.description) return d.description;
  if (d.kind === "webhook") return d.config?.url ?? "";
  return `${serviceLabel} service`;
}

function whereOf(scope: EventScope, projectName?: string, agentName?: string): string {
  if (scope === "org") return "organization";
  if (scope === "project") return `project ${projectName}`;
  return `agent ${projectName}/${agentName}`;
}

/** Lists who still sends to a destination, in place of deleting it. */
function InUseDialog({
  orgName,
  destination,
  onClose,
}: {
  orgName?: string;
  destination: EventDestinationResponse;
  onClose: () => void;
}) {
  const { data, isLoading } = useEventDestination(orgName, destination.id);
  const using = data?.subscriptions ?? [];
  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{destination.name} is in use</DialogTitle>
      <DialogContent>
        <Stack spacing={1.5}>
          <Typography variant="body2">
            These subscriptions send events to it. Delete them, or move them to another destination,
            before deleting it.
          </Typography>
          {isLoading ? (
            <Skeleton variant="rounded" height={64} />
          ) : (
            <Stack component="ul" spacing={0.5} sx={{ m: 0, pl: 2.5 }}>
              {using.map((s) => (
                <li key={s.id}>
                  <Typography variant="body2">
                    <strong>{s.name}</strong>{" "}
                    <Typography component="span" variant="body2" color="text.secondary">
                      ({whereOf(s.scope, s.projectName, s.agentName)})
                    </Typography>
                  </Typography>
                </li>
              ))}
            </Stack>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}

export interface DestinationsTabProps {
  orgName?: string;
  canManage: boolean;
  /** Opens the page that adds a destination. */
  onAdd: () => void;
  /** Opens the page that edits a destination. */
  onEdit: (destination: EventDestinationResponse) => void;
  /** Opens the delivery log filtered to a destination. */
  onViewDeliveries: (destinationId: string) => void;
}

/** The organization's destinations: where events can be sent, and how that is going. */
export function DestinationsTab({
  orgName,
  canManage,
  onAdd,
  onEdit,
  onViewDeliveries,
}: DestinationsTabProps) {
  const { data, isLoading, error } = useListEventDestinations(orgName);
  const { data: kindList } = useDestinationKinds(orgName);
  const services = useMemo(
    () => kindList?.kinds.find((k) => k.kind === "notify")?.services ?? [],
    [kindList],
  );
  const { mutate: remove } = useDeleteEventDestination(orgName);
  const { mutate: rotate } = useRotateEventDestinationSecret(orgName);
  const { mutate: sendTest, isPending: testing } = useTestEventDestination(orgName);
  const { addConfirmation } = useConfirmationDialog();

  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [menu, setMenu] = useState<{
    anchor: HTMLElement;
    destination: EventDestinationResponse;
  } | null>(null);
  const [inUse, setInUse] = useState<EventDestinationResponse | null>(null);
  const [secret, setSecret] = useState<{ name: string; secret: string } | null>(null);
  const [testResult, setTestResult] = useState<{
    name: string;
    result: EventDestinationTestResponse;
  } | null>(null);

  const destinations = useMemo(() => data?.destinations ?? [], [data]);
  const typeOf = (d: EventDestinationResponse) =>
    d.kind === "notify" ? (d.service ?? "notify") : "webhook";
  const typeOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const d of destinations) {
      seen.set(typeOf(d), lookOf(d.kind, d.service, services).label);
    }
    return [...seen.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [destinations, services]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return destinations.filter((d) => {
      if (typeFilter !== "all" && typeOf(d) !== typeFilter) return false;
      if (!q) return true;
      return (
        d.name.toLowerCase().includes(q) ||
        d.description.toLowerCase().includes(q) ||
        lookOf(d.kind, d.service, services).label.toLowerCase().includes(q)
      );
    });
  }, [destinations, search, typeFilter, services]);

  const handleDelete = (d: EventDestinationResponse) => {
    if (d.usedBy > 0) {
      setInUse(d);
      return;
    }
    addConfirmation({
      analytics: { entity: "event-destination", action: "delete" },
      title: "Delete destination",
      description: `"${d.name}" will be removed, with its delivery history. No subscription uses it.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash2 size={16} />,
      onConfirm: () => remove(d.id),
    });
  };

  const handleRotate = (d: EventDestinationResponse) =>
    addConfirmation({
      analytics: { entity: "event-destination", action: "rotate-secret" },
      title: "Rotate signing secret",
      description:
        "A new secret is generated and the current one stops working immediately. " +
        "Update the receiver before the next event is sent.",
      confirmButtonText: "Rotate",
      onConfirm: () =>
        rotate(d.id, { onSuccess: (res) => setSecret({ name: d.name, secret: res.secret }) }),
    });

  const handleTest = (d: EventDestinationResponse) => {
    setTestResult(null);
    sendTest(d.id, { onSuccess: (result) => setTestResult({ name: d.name, result }) });
  };

  const isFiltered = !!search.trim() || typeFilter !== "all";
  const addButton = canManage ? (
    <Button
      variant="contained"
      startIcon={<Plus size={16} />}
      onClick={onAdd}
      sx={{ whiteSpace: "nowrap", flexShrink: 0 }}
    >
      Add destination
    </Button>
  ) : undefined;

  // The list sits in a tab, so its add button is in the toolbar beside the filters.
  const toolbar = (
    <Stack
      direction={{ xs: "column", md: "row" }}
      spacing={1.5}
      alignItems={{ xs: "stretch", md: "center" }}
    >
      <SearchBar
        placeholder="Search destinations"
        size="small"
        fullWidth
        value={search}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
      />
      <Box sx={{ minWidth: 170 }}>
        <TextInput
          select
          size="small"
          aria-label="Filter by type"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          slotProps={{
            select: {
              renderValue: (v) =>
                `Type: ${v === "all" ? "All" : (typeOptions.find(([id]) => id === v)?.[1] ?? String(v))}`,
            },
          }}
        >
          <MenuItem value="all">All</MenuItem>
          {typeOptions.map(([id, label]) => (
            <MenuItem key={id} value={id}>
              {label}
            </MenuItem>
          ))}
        </TextInput>
      </Box>
      {addButton}
    </Stack>
  );

  const current = menu?.destination;
  return (
    <Stack spacing={2}>
      {secret && (
        <SecretReveal name={secret.name} secret={secret.secret} onClose={() => setSecret(null)} />
      )}
      {testResult && (
        <Alert
          severity={testResult.result.delivered ? "success" : "error"}
          onClose={() => setTestResult(null)}
        >
          {testResult.result.delivered
            ? `Test event delivered to ${testResult.name}` +
              (testResult.result.statusCode ? ` (HTTP ${testResult.result.statusCode}).` : ".")
            : `Test event to ${testResult.name} failed: ${testResult.result.error ?? "unknown error"}`}
        </Alert>
      )}
      <ResourceListShell
        resourceName="event-destinations"
        disablePaper
        searchValue={search}
        onSearchChange={setSearch}
        toolbar={toolbar}
        error={error ? new Error("Couldn't load destinations. Reload the page to try again.") : undefined}
        isLoading={isLoading}
        isEmpty={isListEmpty({ isLoading, count: destinations.length, isFiltered })}
        isSearchEmpty={!isLoading && destinations.length > 0 && filtered.length === 0}
        emptyState={{
          illustration: <Send size={64} />,
          title: "No destinations yet",
          description:
            "Add a Slack channel, a PagerDuty service or a webhook, then point subscriptions at it.",
          action: addButton,
        }}
        searchEmptyState={{
          illustration: <Send size={64} />,
          title: "No destinations match your filters",
          description: "Try other search keywords or another type.",
        }}
      >
    <ListingTable>
      <ListingTable.Head>
        <ListingTable.Row>
          <ListingTable.Cell>Destination</ListingTable.Cell>
          <ListingTable.Cell>Type</ListingTable.Cell>
          <ListingTable.Cell>Available to</ListingTable.Cell>
          <ListingTable.Cell>Used by</ListingTable.Cell>
          <ListingTable.Cell>Last delivery</ListingTable.Cell>
          <ListingTable.Cell>Status</ListingTable.Cell>
          <ListingTable.Cell align="right" />
        </ListingTable.Row>
      </ListingTable.Head>
      <ListingTable.Body>
        {filtered.map((d) => {
          const look = lookOf(d.kind, d.service, services);
          return (
            <ListingTable.Row
              key={d.id}
              hover
              sx={{ cursor: canManage ? "pointer" : "default" }}
              onClick={() => canManage && onEdit(d)}
            >
              <ListingTable.Cell>
                <Stack
                  direction="row"
                  spacing={1.5}
                  alignItems="center"
                  sx={{ minWidth: 0, maxWidth: 360 }}
                >
                  <DestinationIcon kind={d.kind} service={d.service} size={28} />
                  <Box sx={{ minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={600} noWrap title={d.name}>
                      {d.name}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      noWrap
                      display="block"
                      title={subtitleOf(d, look.label)}
                    >
                      {subtitleOf(d, look.label)}
                    </Typography>
                  </Box>
                </Stack>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2">{look.label}</Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" title={d.projects.join(", ")}>
                  {availabilityLabel(d.availability, d.projects)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2">
                  {d.usedBy === 1 ? "1 subscription" : `${d.usedBy} subscriptions`}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {timeAgo(d.lastDeliveryAt)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <HealthChip health={d.health} failures={d.consecutiveFailures} />
              </ListingTable.Cell>
              <ListingTable.Cell align="right" onClick={(e) => e.stopPropagation()}>
                <Tooltip title="Actions">
                  <IconButton
                    size="small"
                    aria-label={`Actions for ${d.name}`}
                    onClick={(e) => setMenu({ anchor: e.currentTarget, destination: d })}
                  >
                    <MoreHorizontal size={18} />
                  </IconButton>
                </Tooltip>
              </ListingTable.Cell>
            </ListingTable.Row>
          );
        })}
      </ListingTable.Body>
    </ListingTable>
      </ResourceListShell>

      <Menu anchorEl={menu?.anchor} open={!!menu} onClose={() => setMenu(null)}>
        {canManage && (
          <MenuItem
            onClick={() => {
              setMenu(null);
              if (current) onEdit(current);
            }}
          >
            Edit
          </MenuItem>
        )}
        {canManage && (
          <MenuItem
            disabled={testing}
            onClick={() => {
              setMenu(null);
              if (current) handleTest(current);
            }}
          >
            Send test event
          </MenuItem>
        )}
        {canManage && current?.kind === "webhook" && (
          <MenuItem
            onClick={() => {
              setMenu(null);
              if (current) handleRotate(current);
            }}
          >
            Rotate signing secret
          </MenuItem>
        )}
        <MenuItem
          onClick={() => {
            setMenu(null);
            if (current) onViewDeliveries(current.id);
          }}
        >
          View deliveries
        </MenuItem>
        {canManage && (
          <MenuItem
            sx={{ color: "error.main" }}
            onClick={() => {
              setMenu(null);
              if (current) handleDelete(current);
            }}
          >
            Delete
          </MenuItem>
        )}
      </Menu>

      {inUse && (
        <InUseDialog orgName={orgName} destination={inUse} onClose={() => setInUse(null)} />
      )}
    </Stack>
  );
}

export default DestinationsTab;
