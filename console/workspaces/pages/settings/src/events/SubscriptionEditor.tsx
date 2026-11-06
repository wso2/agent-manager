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

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  FormControlLabel,
  Form,
  Radio,
  RadioGroup,
  Skeleton,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, Send } from "@wso2/oxygen-ui-icons-react";
import { useNavigate, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import {
  usePipelineEnvironments,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  useAvailableEventDestinations,
  useCreateEventSubscription,
  useDestinationKinds,
  useEventTypes,
  useListEnvironments,
  useListEventDestinations,
  useListEventSubscriptions,
  useUpdateEventSubscription,
} from "@agent-management-platform/api-client";
import {
  ALL_EVENTS,
  type DestinationAvailability,
  type EventTargetParams,
  type EventType,
} from "@agent-management-platform/types";
import { DestinationDrawer } from "./DestinationDrawer";
import { DestinationPickerDrawer, type PickableDestination } from "./DestinationPickerDrawer";
import {
  availabilityLabel,
  DestinationIcon,
  HealthChip,
  lookOf,
  PAGING_SERVICES,
} from "./eventsUi";
import type { EventsAccess } from "./useEventsAccess";

const NAME_MAX_LENGTH = 100;
const DESCRIPTION_MAX_LENGTH = 500;

/** The "All events" option of the events picker. */
const ALL_EVENTS_OPTION: EventType = {
  type: ALL_EVENTS,
  scope: "org",
  category: "Everything",
  description: "Every event this level can receive, including event types added later.",
};

export interface SubscriptionEditorProps {
  target: EventTargetParams;
  access: EventsAccess;
  /** Where the list is, to go back to. */
  listPath: string;
  /** Names the level in the summary: "refund-agent", "payments" or the organization. */
  levelName: string;
}

/** Create or edit a subscription: which events, to which destination, filtered how. */
export function SubscriptionEditor({
  target,
  access,
  listPath,
  levelName,
}: SubscriptionEditorProps) {
  const navigate = useNavigate();
  const { subscriptionId } = useParams<{ subscriptionId: string }>();
  const { data: list, isLoading: loadingList } = useListEventSubscriptions(target);
  const editing = subscriptionId
    ? list?.subscriptions.find((s) => s.id === subscriptionId)
    : undefined;

  const { data: catalog, isLoading: loadingTypes } = useEventTypes(target.orgName, target.scope);
  const { data: kindList } = useDestinationKinds(target.orgName);
  const services = useMemo(
    () => kindList?.kinds.find((k) => k.kind === "notify")?.services ?? [],
    [kindList],
  );
  const isOrg = target.scope === "org";
  const orgDestinations = useListEventDestinations(target.orgName, isOrg);
  const availableDestinations = useAvailableEventDestinations(target, !isOrg);
  const destinations = useMemo<PickableDestination[]>(() => {
    if (isOrg) {
      return (orgDestinations.data?.destinations ?? []).map((d) => ({
        id: d.id,
        name: d.name,
        kind: d.kind,
        service: d.service,
        health: d.health,
        description: d.description,
        availableTo: availabilityLabel(d.availability, d.projects),
      }));
    }
    return availableDestinations.data?.destinations ?? [];
  }, [isOrg, orgDestinations.data, availableDestinations.data]);
  const loadingDestinations = isOrg ? orgDestinations.isLoading : availableDestinations.isLoading;

  const pipelineEnvs = usePipelineEnvironments(
    isOrg ? undefined : target.orgName,
    isOrg ? undefined : target.projName,
  );
  const { data: orgEnvs } = useListEnvironments({ orgName: isOrg ? target.orgName : undefined });
  const environments = useMemo(
    () =>
      (isOrg ? (orgEnvs ?? []) : pipelineEnvs).map((e) => ({
        name: e.name,
        label: e.displayName ?? e.name,
      })),
    [isOrg, orgEnvs, pipelineEnvs],
  );

  const { mutate: create, isPending: creating } = useCreateEventSubscription(target);
  const { mutate: update, isPending: updating } = useUpdateEventSubscription(target);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [eventTypes, setEventTypes] = useState<string[]>([]);
  const [destinationId, setDestinationId] = useState<string | undefined>();
  const [envMode, setEnvMode] = useState<"all" | "selected">("all");
  const [envs, setEnvs] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [submitted, setSubmitted] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [adding, setAdding] = useState(false);

  // Filled once per subscription: the list refetches after any change (an
  // inline destination added, say), which must not wipe what is being typed.
  const filledFor = useRef<string | undefined>(undefined);
  useEffect(() => {
    const key = editing?.id ?? (subscriptionId ? undefined : "new");
    if (!key || filledFor.current === key) return;
    filledFor.current = key;
    setName(editing?.name ?? "");
    setDescription(editing?.description ?? "");
    setEventTypes(editing?.eventTypes ?? []);
    setDestinationId(editing?.destination.id);
    setEnvMode(editing && editing.environments.length > 0 ? "selected" : "all");
    setEnvs(editing?.environments ?? []);
    setEnabled(editing?.enabled ?? true);
    setSubmitted(false);
  }, [editing, subscriptionId]);

  const effectiveEnvs = envMode === "all" ? [] : envs;
  const isDirty =
    name !== (editing?.name ?? "") ||
    description !== (editing?.description ?? "") ||
    eventTypes.join("\n") !== (editing?.eventTypes ?? []).join("\n") ||
    destinationId !== editing?.destination.id ||
    effectiveEnvs.join("\n") !== (editing?.environments ?? []).join("\n") ||
    enabled !== (editing?.enabled ?? true);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);
  const backToList = () => navigate(`${listPath}?tab=subscriptions`);

  const options = useMemo(() => [ALL_EVENTS_OPTION, ...(catalog?.eventTypes ?? [])], [catalog]);
  const selectedOptions = useMemo(
    () =>
      eventTypes.map(
        (t) =>
          options.find((o) => o.type === t) ?? {
            type: t,
            scope: target.scope,
            category: "No longer offered",
            description: "Not an event this level can receive any more; it is ignored.",
          },
      ),
    [eventTypes, options, target.scope],
  );
  const allEvents = eventTypes.length === 1 && eventTypes[0] === ALL_EVENTS;

  const destination =
    destinations.find((d) => d.id === destinationId) ??
    (editing && editing.destination.id === destinationId
      ? { ...editing.destination, description: undefined, availableTo: undefined }
      : undefined);
  const destinationLook = destination
    ? lookOf(destination.kind, destination.service, services)
    : undefined;

  const defaultAvailability = useMemo<{
    availability: DestinationAvailability;
    projects: string[];
  }>(
    () =>
      isOrg || !target.projName
        ? { availability: "all", projects: [] }
        : { availability: "projects", projects: [target.projName] },
    [isOrg, target.projName],
  );

  const nameError = submitted && !name.trim() ? "Enter a name" : undefined;
  const eventsError =
    submitted && eventTypes.length === 0 ? "Choose at least one event" : undefined;
  const destinationError = submitted && !destinationId ? "Choose where the events go" : undefined;
  const envError =
    submitted && envMode === "selected" && envs.length === 0
      ? "Select at least one environment"
      : undefined;

  const handleSubmit = () => {
    setSubmitted(true);
    if (!name.trim() || eventTypes.length === 0 || !destinationId) return;
    if (envMode === "selected" && envs.length === 0) return;
    const body = {
      name: name.trim(),
      description: description.trim(),
      destinationId,
      eventTypes,
      environments: effectiveEnvs,
      enabled,
    };
    const done = { onSuccess: () => allowNavigation(backToList) };
    if (editing) {
      update({ id: editing.id, body }, done);
    } else {
      create(body, done);
    }
  };

  const title = subscriptionId ? "Edit subscription" : "New subscription";
  if (subscriptionId && !editing) {
    return (
      <PageLayout
        title={title}
        disableIcon
        backHref={`${listPath}?tab=subscriptions`}
        backLabel="Back to Events"
      >
        {loadingList ? (
          <Skeleton variant="rounded" height={320} />
        ) : (
          <Alert severity="error">
            This subscription no longer exists, or belongs to another level.
          </Alert>
        )}
      </PageLayout>
    );
  }

  const summaryEvents = allEvents
    ? "every event"
    : eventTypes.length === 0
      ? "the events you choose"
      : eventTypes.length === 1
        ? eventTypes[0]
        : `${eventTypes[0]} and ${eventTypes.length - 1} more`;
  const summaryWhere = isOrg ? "anywhere in the organization" : `for ${levelName}`;
  const summaryEnvs =
    envMode === "selected" && envs.length > 0
      ? ` in ${envs.map((e) => environments.find((x) => x.name === e)?.label ?? e).join(", ")}`
      : "";
  const pagingAll =
    allEvents && !!destination?.service && PAGING_SERVICES.includes(destination.service);

  return (
    <PageLayout
      title={title}
      description={
        isOrg
          ? "Organization subscriptions send the events of every project and agent."
          : `Choose which of ${levelName}'s events to send, and where.`
      }
      disableIcon
      backHref={`${listPath}?tab=subscriptions`}
      backLabel="Back to Events"
    >
      <Stack spacing={3} sx={{ maxWidth: 880 }}>
        <Form.Stack>
          <Form.Section>
            <Form.Header>Details</Form.Header>
            <Form.ElementWrapper name="name" label="Name">
              <TextField
                id="name"
                required
                fullWidth
                placeholder="Deploys to team channel"
                slotProps={{ htmlInput: { maxLength: NAME_MAX_LENGTH } }}
                value={name}
                onChange={(e) => setName(e.target.value)}
                error={!!nameError}
                helperText={nameError}
              />
            </Form.ElementWrapper>
            <Form.ElementWrapper name="description" label="Description">
              <TextField
                id="description"
                fullWidth
                multiline
                minRows={2}
                placeholder="Optional"
                slotProps={{ htmlInput: { maxLength: DESCRIPTION_MAX_LENGTH } }}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </Form.ElementWrapper>
          </Form.Section>

          <Form.Section>
            <Form.Header>Events</Form.Header>
            <Typography variant="caption" color="text.secondary">
              {target.scope === "agent"
                ? "The agent's events: builds, deployments, configuration, API keys, monitors and monitor runs."
                : target.scope === "project"
                  ? "The project's events and those of every agent in it."
                  : "Any event in the organization, from projects, agents and shared resources."}
            </Typography>
            {loadingTypes ? (
              <Skeleton variant="rounded" height={56} />
            ) : (
              <Autocomplete
                multiple
                disableCloseOnSelect
                options={options}
                value={selectedOptions}
                groupBy={(o) => o.category}
                getOptionLabel={(o) => (o.type === ALL_EVENTS ? "All events" : o.type)}
                isOptionEqualToValue={(a, b) => a.type === b.type}
                onChange={(_, value) => {
                  const types = value.map((o) => o.type);
                  const added = types.find((t) => !eventTypes.includes(t));
                  // "All events" stands alone: choosing it replaces the rest,
                  // choosing anything else replaces it.
                  if (added === ALL_EVENTS) setEventTypes([ALL_EVENTS]);
                  else setEventTypes(types.filter((t) => t !== ALL_EVENTS));
                }}
                renderOption={(props, option, { selected }) => {
                  const { key, ...rest } = props as typeof props & { key: string };
                  return (
                    <li key={key} {...rest}>
                      <Checkbox size="small" checked={selected} sx={{ mr: 1 }} />
                      <Box sx={{ minWidth: 0 }}>
                        <Typography
                          variant="body2"
                          sx={{ fontFamily: option.type === ALL_EVENTS ? undefined : "monospace" }}
                          fontWeight={option.type === ALL_EVENTS ? 600 : 400}
                        >
                          {option.type === ALL_EVENTS ? "All events (*)" : option.type}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {option.description}
                        </Typography>
                      </Box>
                    </li>
                  );
                }}
                renderTags={(value, getTagProps) =>
                  value.map((o, index) => {
                    const { key, ...tagProps } = getTagProps({ index });
                    return (
                      <Chip
                        key={key}
                        size="small"
                        label={o.type === ALL_EVENTS ? "* all events" : o.type}
                        sx={{ fontFamily: "monospace" }}
                        {...tagProps}
                      />
                    );
                  })
                }
                renderInput={(params) => (
                  <TextField
                    {...params}
                    placeholder={eventTypes.length ? "" : "Choose events"}
                    error={!!eventsError}
                    helperText={eventsError}
                  />
                )}
              />
            )}
          </Form.Section>

          <Form.Section>
            <Form.Header>Destination</Form.Header>
            <Typography variant="caption" color="text.secondary">
              Where the events go. Organization admins register destinations; you pick one available
              to {isOrg ? "the organization" : "this project"}.
            </Typography>
            {destination ? (
              <Card variant="outlined">
                <CardContent
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1.5,
                    "&:last-child": { pb: 2 },
                  }}
                >
                  <DestinationIcon
                    kind={destination.kind}
                    service={destination.service}
                    size={34}
                  />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="subtitle1" fontWeight={600} noWrap>
                      {destination.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" noWrap display="block">
                      {[
                        destinationLook?.label,
                        destination.availableTo && `Available to ${destination.availableTo}`,
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </Typography>
                  </Box>
                  <HealthChip health={destination.health} />
                  <Button
                    variant="text"
                    startIcon={<Pencil size={14} />}
                    onClick={() => setPickerOpen(true)}
                  >
                    Change
                  </Button>
                </CardContent>
              </Card>
            ) : (
              <Box>
                <Button
                  variant="outlined"
                  startIcon={<Send size={16} />}
                  onClick={() => setPickerOpen(true)}
                >
                  Select destination
                </Button>
                {destinationError && (
                  <Typography variant="caption" color="error" display="block" sx={{ mt: 0.5 }}>
                    {destinationError}
                  </Typography>
                )}
              </Box>
            )}
            {pagingAll && (
              <Alert severity="warning">
                {destinationLook?.label} pages people. Sending every event to it can wake someone
                for routine changes; consider choosing the failure events only.
              </Alert>
            )}
          </Form.Section>

          <Form.Section>
            <Form.Header>Filter</Form.Header>
            <RadioGroup
              row
              value={envMode}
              onChange={(e) => setEnvMode(e.target.value as "all" | "selected")}
            >
              <FormControlLabel
                value="all"
                control={<Radio size="small" />}
                label="All environments"
              />
              <FormControlLabel
                value="selected"
                control={<Radio size="small" />}
                label="Selected environments"
              />
            </RadioGroup>
            {envMode === "selected" && (
              <Stack direction="row" flexWrap="wrap" gap={1}>
                {environments.map((env) => {
                  const on = envs.includes(env.name);
                  return (
                    <Chip
                      key={env.name}
                      label={env.label}
                      color={on ? "primary" : "default"}
                      variant={on ? "filled" : "outlined"}
                      onClick={() =>
                        setEnvs((current) =>
                          on ? current.filter((e) => e !== env.name) : [...current, env.name],
                        )
                      }
                    />
                  );
                })}
                {environments.length === 0 && (
                  <Typography variant="body2" color="text.secondary">
                    No environments found.
                  </Typography>
                )}
              </Stack>
            )}
            {envError && (
              <Typography variant="caption" color="error">
                {envError}
              </Typography>
            )}
            <Typography variant="caption" color="text.secondary">
              Events not tied to an environment, such as builds and project changes, are always
              sent.
            </Typography>
          </Form.Section>

          <Form.Section>
            <Form.Header>Status</Form.Header>
            <FormControlLabel
              control={<Switch checked={enabled} onChange={(_, v) => setEnabled(v)} />}
              label={enabled ? "Enabled: events are sent" : "Disabled: nothing is sent"}
            />
          </Form.Section>
        </Form.Stack>

        <Alert severity="info" icon={<Send size={18} />}>
          Send <strong>{summaryEvents}</strong> {summaryWhere}
          {summaryEnvs} to <strong>{destination?.name ?? "the destination you pick"}</strong>
          {enabled ? "." : ", once enabled."}
        </Alert>

        <Stack direction="row" gap={2}>
          <Button variant="outlined" onClick={backToList} disabled={creating || updating}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSubmit}
            disabled={creating || updating || !access.canManageSubscriptions}
          >
            {editing ? "Save" : "Create subscription"}
          </Button>
        </Stack>
      </Stack>

      <DestinationPickerDrawer
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        destinations={destinations}
        services={services}
        isLoading={loadingDestinations}
        selectedId={destinationId}
        onSelect={setDestinationId}
        onAdd={
          access.canManageDestinations
            ? () => {
                setPickerOpen(false);
                setAdding(true);
              }
            : undefined
        }
      />
      <DestinationDrawer
        open={adding}
        orgName={target.orgName}
        defaultAvailability={defaultAvailability}
        createLabel="Add & use"
        onClose={() => setAdding(false)}
        onSaved={(saved) => setDestinationId(saved.id)}
      />
    </PageLayout>
  );
}

export default SubscriptionEditor;
