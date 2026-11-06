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

import { useEffect, useMemo, useState } from "react";
import {
  Autocomplete,
  Box,
  Button,
  FormControlLabel,
  Grid,
  Link,
  ListSubheader,
  MenuItem,
  Radio,
  RadioGroup,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { MoreHorizontal } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";
import { useDestinationKinds, useListAllProjects } from "@agent-management-platform/api-client";
import {
  globalConfig,
  type DestinationAvailability,
  type DestinationKind,
  type DestinationKindService,
  type EventDestinationRequest,
  type EventDestinationResponse,
} from "@agent-management-platform/types";
import {
  DestinationIcon,
  EVENT_DESCRIPTION_MAX_LENGTH,
  EVENT_NAME_MAX_LENGTH,
  FEATURED_SERVICES,
  lookOf,
  SectionTitle,
} from "./eventsUi";

const alertingDocLinks = globalConfig.alertingDocLinks;
const webhookGuideUrl =
  globalConfig.docsUrl && alertingDocLinks?.webhookDestination
    ? `${globalConfig.docsUrl}${alertingDocLinks.webhookDestination}`
    : undefined;

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

/** A choice in the type picker: a webhook, or one notification service. */
interface TypeChoice {
  kind: DestinationKind;
  service: string;
}

export interface DestinationFormProps {
  orgName?: string;
  /** The destination being edited; undefined to add one. */
  destination?: EventDestinationResponse;
  /** Availability a new destination starts with. */
  defaultAvailability?: { availability: DestinationAvailability; projects: string[] };
  saving: boolean;
  submitLabel: string;
  onCancel: () => void;
  onSubmit: (body: EventDestinationRequest) => void;
  onDirtyChange?: (dirty: boolean) => void;
}

/** One card of the type picker. */
function TypeCard({
  label,
  icon,
  selected,
  disabled,
  onClick,
}: {
  label: string;
  icon: React.ReactNode;
  selected: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <Box
      component="button"
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-pressed={selected}
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1.25,
        px: 1.5,
        py: 1.25,
        borderRadius: 1.5,
        border: 1,
        borderColor: selected ? "primary.main" : "divider",
        bgcolor: selected ? "action.selected" : "background.paper",
        boxShadow: selected ? (t) => `0 0 0 1px ${t.palette.primary.main}` : "none",
        cursor: disabled ? "default" : "pointer",
        opacity: disabled && !selected ? 0.5 : 1,
        textAlign: "left",
        font: "inherit",
        color: "text.primary",
        minWidth: 0,
        "&:hover": disabled ? {} : { borderColor: "primary.main" },
      }}
    >
      {icon}
      <Typography variant="body2" fontWeight={selected ? 600 : 400} noWrap>
        {label}
      </Typography>
    </Box>
  );
}

/** Add or edit a destination: its type, connection, name and who may use it. */
export function DestinationForm({
  orgName,
  destination,
  defaultAvailability,
  saving,
  submitLabel,
  onCancel,
  onSubmit,
  onDirtyChange,
}: DestinationFormProps) {
  const { data: kindList } = useDestinationKinds(orgName);
  const { data: projectList } = useListAllProjects({ orgName });
  const services = useMemo<DestinationKindService[]>(
    () => kindList?.kinds.find((k) => k.kind === "notify")?.services ?? [],
    [kindList],
  );
  const notifyOffered = !kindList || kindList.kinds.some((k) => k.kind === "notify");
  const webhookOffered = !kindList || kindList.kinds.some((k) => k.kind === "webhook");

  const initial = useMemo(
    () => ({
      type: {
        kind: destination?.kind ?? (notifyOffered ? "notify" : "webhook"),
        service: destination?.service ?? (destination ? "" : "slack"),
      } as TypeChoice,
      name: destination?.name ?? "",
      description: destination?.description ?? "",
      // A notification service URL is a credential and never comes back.
      url: destination?.kind === "webhook" ? (destination.config?.url ?? "") : "",
      availability: destination?.availability ?? defaultAvailability?.availability ?? "all",
      projects: destination?.projects ?? defaultAvailability?.projects ?? [],
    }),
    [destination, defaultAvailability, notifyOffered],
  );

  const [type, setType] = useState<TypeChoice>(initial.type);
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [url, setUrl] = useState(initial.url);
  const [availability, setAvailability] = useState<DestinationAvailability>(initial.availability);
  const [projects, setProjects] = useState<string[]>(initial.projects);
  const [showMore, setShowMore] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    setType(initial.type);
    setName(initial.name);
    setDescription(initial.description);
    setUrl(initial.url);
    setAvailability(initial.availability);
    setProjects(initial.projects);
    setShowMore(!!initial.type.service && !FEATURED_SERVICES.includes(initial.type.service));
    setSubmitted(false);
  }, [initial]);

  const isDirty =
    type.kind !== initial.type.kind ||
    type.service !== initial.type.service ||
    name !== initial.name ||
    description !== initial.description ||
    url !== initial.url ||
    availability !== initial.availability ||
    projects.join("\n") !== initial.projects.join("\n");
  useEffect(() => {
    onDirtyChange?.(isDirty);
  }, [isDirty, onDirtyChange]);

  const editing = !!destination;
  const isNotify = type.kind === "notify";
  const selectedService = services.find((s) => s.id === type.service);
  const serviceGuidePath = alertingDocLinks?.notificationServices?.[type.service];
  const serviceGuideUrl =
    globalConfig.docsUrl && serviceGuidePath
      ? `${globalConfig.docsUrl}${serviceGuidePath}`
      : undefined;
  // Editing a notification destination keeps its URL unless a new one is
  // entered; a new destination, or another service, needs one.
  const keepsUrl =
    isNotify && editing && type.service === (destination?.service ?? "");
  const moreServices = services.filter(
    (s) => !FEATURED_SERVICES.includes(s.id),
  );
  const moreGroups = useMemo(() => {
    const byCategory = new Map<string, DestinationKindService[]>();
    for (const s of moreServices) {
      byCategory.set(s.category, [...(byCategory.get(s.category) ?? []), s]);
    }
    return [...byCategory.entries()];
  }, [moreServices]);

  const urlProblem = (): string | undefined => {
    const v = url.trim();
    if (!isNotify) return isHttpUrl(v) ? undefined : "Enter an http(s) URL";
    if (!v) return keepsUrl ? undefined : "Enter the service URL";
    const scheme = v.split("://")[0]?.split("+")[0];
    return scheme === type.service
      ? undefined
      : `A ${selectedService?.label ?? type.service} URL starts with ${type.service}://`;
  };
  const nameError = submitted && !name.trim() ? "Enter a name" : undefined;
  const serviceError = submitted && isNotify && !type.service ? "Choose a service" : undefined;
  const urlError = submitted && (!isNotify || !!type.service) ? urlProblem() : undefined;
  const projectsError =
    submitted && availability === "projects" && projects.length === 0
      ? "Select at least one project"
      : undefined;

  const choose = (choice: TypeChoice) => {
    if (editing) return;
    setType(choice);
    setUrl("");
  };

  const handleSubmit = () => {
    setSubmitted(true);
    if (!name.trim() || (isNotify && !type.service) || urlProblem()) return;
    if (availability === "projects" && projects.length === 0) return;
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      kind: type.kind,
      config: isNotify
        ? { service: type.service, ...(url.trim() ? { url: url.trim() } : {}) }
        : { url: url.trim() },
      availability,
      projects: availability === "projects" ? projects : [],
    });
  };

  const projectOptions = useMemo(
    () => (projectList?.projects ?? []).map((p) => p.name).sort(),
    [projectList],
  );

  return (
    <Stack spacing={3}>
      <Stack spacing={1.5}>
        <SectionTitle hint={editing ? "The type of a destination cannot be changed." : undefined}>
          Type
        </SectionTitle>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(170px, 1fr))",
            gap: 1,
          }}
        >
          {notifyOffered &&
            FEATURED_SERVICES.map((id) => {
              const look = lookOf("notify", id, services);
              const selected = isNotify && type.service === id;
              return (
                <TypeCard
                  key={id}
                  label={look.label}
                  icon={<DestinationIcon kind="notify" service={id} size={26} />}
                  selected={selected}
                  disabled={editing && !selected}
                  onClick={() => {
                    setShowMore(false);
                    choose({ kind: "notify", service: id });
                  }}
                />
              );
            })}
          {webhookOffered && (
            <TypeCard
              label="Webhook"
              icon={<DestinationIcon kind="webhook" size={26} />}
              selected={!isNotify}
              disabled={editing && isNotify}
              onClick={() => {
                setShowMore(false);
                choose({ kind: "webhook", service: "" });
              }}
            />
          )}
          {notifyOffered && moreServices.length > 0 && (
            <TypeCard
              label="More services"
              icon={
                <Box
                  sx={{
                    width: 26,
                    height: 26,
                    borderRadius: 1,
                    border: 1,
                    borderColor: "divider",
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    color: "text.secondary",
                  }}
                >
                  <MoreHorizontal size={16} />
                </Box>
              }
              selected={showMore}
              disabled={editing && !showMore}
              onClick={() => {
                if (editing) return;
                setShowMore(true);
                setType({ kind: "notify", service: "" });
                setUrl("");
              }}
            />
          )}
        </Box>
        {showMore && (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                select
                label="Service"
                value={type.service}
                disabled={editing}
                onChange={(e) => choose({ kind: "notify", service: e.target.value })}
                error={!!serviceError}
                helperText={serviceError ?? "Choose a service to configure its connection."}
                slotProps={{
                  select: {
                    displayEmpty: true,
                    renderValue: (value) =>
                      value ? (
                        <Stack direction="row" spacing={1} alignItems="center">
                          <DestinationIcon kind="notify" service={String(value)} size={20} />
                          <span>
                            {services.find((s) => s.id === value)?.label ?? String(value)}
                          </span>
                        </Stack>
                      ) : (
                        <Typography color="text.secondary">Select a service</Typography>
                      ),
                  },
                }}
              >
                {moreGroups.flatMap(([category, items]) => [
                  <ListSubheader key={`h-${category}`} sx={{ lineHeight: "32px" }}>
                    {category}
                  </ListSubheader>,
                  ...items.map((s) => (
                    <MenuItem key={s.id} value={s.id} sx={{ gap: 1, alignItems: "center" }}>
                      <DestinationIcon kind="notify" service={s.id} size={20} />
                      {s.label}
                    </MenuItem>
                  )),
                ])}
              </TextInput>
            </Grid>
          </Grid>
        )}
      </Stack>

      <Stack spacing={1.5}>
        <SectionTitle>Connection</SectionTitle>
        {isNotify && !selectedService ? (
          <Typography variant="body2" color="text.secondary">
            Choose a service first.
          </Typography>
        ) : (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, xl: 8 }}>
              {isNotify && selectedService ? (
                <TextInput
                  label={`${selectedService.label} URL`}
                  autoComplete="off"
                  placeholder={
                    keepsUrl
                      ? "Leave blank to keep the current URL"
                      : `e.g. ${selectedService.urlFormat}`
                  }
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  error={!!urlError}
                  helperText={
                    <>
                      {urlError ??
                        "It holds the service's token, so it is stored encrypted and never shown again."}
                      {serviceGuideUrl && (
                        <>
                          {" "}
                          <Link
                            href={serviceGuideUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Set up this destination
                          </Link>
                        </>
                      )}
                    </>
                  }
                />
              ) : (
                <TextInput
                  label="Endpoint URL"
                  placeholder="e.g. https://ci.example.com/hooks/amp"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  error={!!urlError}
                  helperText={
                    <>
                      {urlError ??
                        "Each event is POSTed here as signed CloudEvents JSON (Standard Webhooks). The signing secret is shown once, after you add it."}
                      {webhookGuideUrl && (
                        <>
                          {" "}
                          <Link
                            href={webhookGuideUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Set up this destination
                          </Link>
                        </>
                      )}
                    </>
                  }
                />
              )}
            </Grid>
          </Grid>
        )}
      </Stack>

      <Stack spacing={1.5}>
        <SectionTitle>Details</SectionTitle>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Name"
              placeholder={isNotify ? "e.g. #platform-alerts" : "e.g. Jenkins automation"}
              maxLength={EVENT_NAME_MAX_LENGTH}
              hideCharacterCount={name.length < EVENT_NAME_MAX_LENGTH - 10}
              value={name}
              onChange={(e) => setName(e.target.value)}
              error={!!nameError}
              helperText={
                nameError ??
                "Shown when picking a destination for a subscription. Unique in the organization."
              }
            />
          </Grid>
          <Grid size={{ xs: 12, xl: 8 }}>
            <TextInput
              label="Description (optional)"
              placeholder={
                isNotify ? "e.g. acme.slack.com, #platform-alerts" : "e.g. Triggers the release pipeline"
              }
              maxLength={EVENT_DESCRIPTION_MAX_LENGTH}
              hideCharacterCount={description.length < EVENT_DESCRIPTION_MAX_LENGTH - 50}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              helperText="Shown under the name."
            />
          </Grid>
        </Grid>
      </Stack>

      <Stack spacing={1}>
        <SectionTitle hint="Organization subscriptions can always use it; this decides which projects and agents can subscribe to it.">
          Available to
        </SectionTitle>
        <RadioGroup
          value={availability}
          onChange={(e) => setAvailability(e.target.value as DestinationAvailability)}
        >
          <FormControlLabel value="all" control={<Radio size="small" />} label="All projects" />
          <FormControlLabel
            value="projects"
            control={<Radio size="small" />}
            label="Selected projects"
          />
          <FormControlLabel
            value="org"
            control={<Radio size="small" />}
            label="Organization only"
          />
        </RadioGroup>
        {availability === "projects" && (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, xl: 8 }}>
              <Autocomplete
                multiple
                options={projectOptions}
                value={projects}
                onChange={(_, value) => setProjects(value)}
                renderInput={(params) => (
                  <TextInput
                    {...params}
                    label="Projects"
                    placeholder={projects.length ? "" : "e.g. payments"}
                    error={!!projectsError}
                    helperText={projectsError}
                  />
                )}
              />
            </Grid>
          </Grid>
        )}
      </Stack>

      <Stack direction="row" gap={1} justifyContent="flex-start">
        <Button variant="outlined" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button variant="contained" onClick={handleSubmit} disabled={saving}>
          {submitLabel}
        </Button>
      </Stack>
    </Stack>
  );
}

export default DestinationForm;
