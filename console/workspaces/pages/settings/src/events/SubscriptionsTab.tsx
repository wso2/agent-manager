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

import { Fragment, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  IconButton,
  Link,
  ListingTable,
  Menu,
  MenuItem,
  Skeleton,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Bell, MoreHorizontal, Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { Link as RouterLink } from "react-router-dom";
import {
  useDeleteEventSubscription,
  useUpdateEventSubscription,
  useUserDisplayName,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import type {
  EventSubscriptionListResponse,
  EventSubscriptionResponse,
  EventTargetParams,
} from "@agent-management-platform/types";
import { DestinationName, EventTypeChips, filterLabel, Last24hChip, timeAgo } from "./eventsUi";

/** "by nuwan.p, 3 days ago", with the creator's name resolved from their ID. */
function CreatedBy({ orgName, userId, at }: { orgName?: string; userId: string; at: string }) {
  const name = useUserDisplayName({ orgName: orgName ?? "", userId });
  return (
    <Typography variant="caption" color="text.secondary" noWrap display="block">
      {userId ? `by ${name ?? "a teammate"}, ${timeAgo(at)}` : `added ${timeAgo(at)}`}
    </Typography>
  );
}

type Show = "all" | "own" | "inherited";

export interface SubscriptionsTabProps {
  target: EventTargetParams;
  data?: EventSubscriptionListResponse;
  isLoading: boolean;
  error?: unknown;
  canManage: boolean;
  /** "agent", "project" or "organization", for the group header. */
  levelNoun: string;
  /** Where inherited rows are managed; links are left out when not given. */
  projectEventsHref?: string;
  orgEventsHref?: string;
  onNew: () => void;
  onEdit: (s: EventSubscriptionResponse) => void;
  onViewDeliveries: (subscriptionId: string) => void;
}

/** A level's subscriptions, with the wider levels' that reach it shown locked. */
export function SubscriptionsTab({
  target,
  data,
  isLoading,
  error,
  canManage,
  levelNoun,
  projectEventsHref,
  orgEventsHref,
  onNew,
  onEdit,
  onViewDeliveries,
}: SubscriptionsTabProps) {
  const {
    mutate: update,
    isPending: toggling,
    variables: toggled,
  } = useUpdateEventSubscription(target);
  const { mutate: remove } = useDeleteEventSubscription(target);
  const { addConfirmation } = useConfirmationDialog();
  const [search, setSearch] = useState("");
  const [show, setShow] = useState<Show>("all");
  const [menu, setMenu] = useState<{
    anchor: HTMLElement;
    subscription: EventSubscriptionResponse;
  } | null>(null);

  const matches = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (s: EventSubscriptionResponse) =>
      !q ||
      s.name.toLowerCase().includes(q) ||
      s.destination.name.toLowerCase().includes(q) ||
      s.eventTypes.some((t) => t.toLowerCase().includes(q));
  }, [search]);

  const own = (data?.subscriptions ?? []).filter(matches);
  const inherited = (data?.inherited ?? []).filter(matches);
  const fromProject = inherited.filter((s) => s.scope === "project");
  const fromOrg = inherited.filter((s) => s.scope === "org");
  const hasInherited = (data?.inherited.length ?? 0) > 0;

  const toggle = (s: EventSubscriptionResponse, enabled: boolean) =>
    update({
      id: s.id,
      body: {
        name: s.name,
        description: s.description,
        destinationId: s.destination.id,
        eventTypes: s.eventTypes,
        environments: s.environments,
        enabled,
      },
    });

  const handleDelete = (s: EventSubscriptionResponse) =>
    addConfirmation({
      analytics: { entity: "event-subscription", action: "delete" },
      title: "Delete subscription",
      description: `"${s.name}" stops sending events to ${s.destination.name}. Events already queued are still sent.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash2 size={16} />,
      onConfirm: () => remove(s.id),
    });

  const groupHeader = (label: string, link?: { href: string; text: string }) => (
    <ListingTable.Row>
      <ListingTable.Cell colSpan={7} sx={{ bgcolor: "action.hover", py: 1 }}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="caption" fontWeight={600} color="text.secondary">
            {label}
          </Typography>
          {link && (
            <Link component={RouterLink} to={link.href} variant="caption" underline="hover">
              {link.text}
            </Link>
          )}
        </Stack>
      </ListingTable.Cell>
    </ListingTable.Row>
  );

  const row = (s: EventSubscriptionResponse, locked: boolean) => (
    <ListingTable.Row
      key={s.id}
      hover={!locked}
      sx={{ cursor: !locked && canManage ? "pointer" : "default" }}
      onClick={() => !locked && canManage && onEdit(s)}
    >
      <ListingTable.Cell>
        <Box sx={{ minWidth: 0, maxWidth: 280 }}>
          <Typography variant="body2" fontWeight={600} noWrap title={s.name}>
            {s.name}
          </Typography>
          {locked ? (
            <Chip
              size="small"
              variant="outlined"
              label={s.scope === "org" ? "From org" : "From project"}
              sx={{ mt: 0.5, height: 20, fontSize: 11 }}
            />
          ) : (
            <CreatedBy orgName={target.orgName} userId={s.createdBy} at={s.createdAt} />
          )}
        </Box>
      </ListingTable.Cell>
      <ListingTable.Cell>
        <EventTypeChips types={s.eventTypes} />
      </ListingTable.Cell>
      <ListingTable.Cell>
        <DestinationName
          kind={s.destination.kind}
          service={s.destination.service}
          name={s.destination.name}
        />
      </ListingTable.Cell>
      <ListingTable.Cell>
        <Typography variant="body2" color="text.secondary" noWrap>
          {filterLabel(s.environments)}
        </Typography>
      </ListingTable.Cell>
      <ListingTable.Cell>
        <Last24hChip counts={s.last24h} />
      </ListingTable.Cell>
      <ListingTable.Cell onClick={(e) => e.stopPropagation()}>
        <Tooltip
          title={
            locked
              ? `Managed in the ${s.scope === "org" ? "organization" : "project"}`
              : s.enabled
                ? "Disable subscription"
                : "Enable subscription"
          }
        >
          <span>
            <Switch
              size="small"
              checked={s.enabled}
              disabled={locked || !canManage || (toggling && toggled?.id === s.id)}
              inputProps={{ "aria-label": `${s.enabled ? "Disable" : "Enable"} ${s.name}` }}
              onChange={(_, v) => toggle(s, v)}
            />
          </span>
        </Tooltip>
      </ListingTable.Cell>
      <ListingTable.Cell align="right" onClick={(e) => e.stopPropagation()}>
        {!locked && (
          <IconButton
            size="small"
            aria-label={`Actions for ${s.name}`}
            onClick={(e) => setMenu({ anchor: e.currentTarget, subscription: s })}
          >
            <MoreHorizontal size={18} />
          </IconButton>
        )}
      </ListingTable.Cell>
    </ListingTable.Row>
  );

  const showOwn = show !== "inherited";
  const showInherited = show !== "own";
  const visibleCount = (showOwn ? own.length : 0) + (showInherited ? inherited.length : 0);

  const toolbar = (
    <ListingTable.Toolbar
      showSearch
      searchValue={search}
      onSearchChange={(v: string) => setSearch(v)}
      searchPlaceholder="Search subscriptions"
      actions={
        <Stack direction="row" spacing={1.5} alignItems="center">
          {hasInherited && (
            <TextField
              select
              size="small"
              value={show}
              onChange={(e) => setShow(e.target.value as Show)}
              sx={{ minWidth: 140 }}
              slotProps={{
                select: {
                  renderValue: (v) =>
                    `Show: ${v === "all" ? "All" : v === "own" ? `This ${levelNoun}` : "Inherited"}`,
                },
              }}
            >
              <MenuItem value="all">All</MenuItem>
              <MenuItem value="own">{`This ${levelNoun}`}</MenuItem>
              <MenuItem value="inherited">Inherited</MenuItem>
            </TextField>
          )}
          {canManage && (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              onClick={onNew}
              sx={{ whiteSpace: "nowrap" }}
            >
              New subscription
            </Button>
          )}
        </Stack>
      }
    />
  );

  let body;
  if (error) {
    body = (
      <Alert severity="error" icon={<AlertTriangle size={18} />}>
        Failed to load subscriptions. Please try again.
      </Alert>
    );
  } else if (isLoading) {
    body = (
      <Stack spacing={1} m={2}>
        <Skeleton variant="rounded" height={56} />
        <Skeleton variant="rounded" height={56} />
      </Stack>
    );
  } else if (visibleCount === 0) {
    const nothing = (data?.subscriptions.length ?? 0) + (data?.inherited.length ?? 0) === 0;
    body = (
      <ListingTable.EmptyState
        illustration={<Bell size={64} />}
        title={nothing ? "No subscriptions yet" : "No subscriptions match"}
        description={
          nothing
            ? `Pick events and a destination to send this ${levelNoun}'s events to Slack, PagerDuty or a webhook.`
            : "Try other search keywords or show all subscriptions."
        }
        action={
          nothing && canManage ? (
            <Button variant="contained" startIcon={<Plus size={16} />} onClick={onNew}>
              New subscription
            </Button>
          ) : undefined
        }
      />
    );
  } else {
    body = (
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Subscription</ListingTable.Cell>
            <ListingTable.Cell>Events</ListingTable.Cell>
            <ListingTable.Cell>Destination</ListingTable.Cell>
            <ListingTable.Cell>Filter</ListingTable.Cell>
            <ListingTable.Cell>Last 24h</ListingTable.Cell>
            <ListingTable.Cell>Enabled</ListingTable.Cell>
            <ListingTable.Cell align="right" />
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {showOwn && own.length > 0 && (
            <Fragment>
              {hasInherited && groupHeader(`This ${levelNoun}`)}
              {own.map((s) => row(s, false))}
            </Fragment>
          )}
          {showInherited && fromProject.length > 0 && (
            <Fragment>
              {groupHeader(
                `Inherited from project ${target.projName}`,
                projectEventsHref
                  ? { href: projectEventsHref, text: "Manage in project" }
                  : undefined,
              )}
              {fromProject.map((s) => row(s, true))}
            </Fragment>
          )}
          {showInherited && fromOrg.length > 0 && (
            <Fragment>
              {groupHeader(
                `Inherited from organization ${target.orgName}`,
                orgEventsHref ? { href: orgEventsHref, text: "View in org" } : undefined,
              )}
              {fromOrg.map((s) => row(s, true))}
            </Fragment>
          )}
        </ListingTable.Body>
      </ListingTable>
    );
  }

  const current = menu?.subscription;
  return (
    <>
      <ListingTable.Container>
        {toolbar}
        {body}
      </ListingTable.Container>
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
    </>
  );
}

export default SubscriptionsTab;
