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

import { Alert, Box, Chip, IconButton, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Copy } from "@wso2/oxygen-ui-icons-react";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import {
  ALL_EVENTS,
  INPUT_LIMITS,
  type DestinationAvailability,
  type DestinationHealth,
  type DestinationKind,
  type DestinationKindService,
  type EventDeliveryCounts,
  type EventDeliveryStatus,
} from "@agent-management-platform/types";

/** Limits the service enforces on destination and subscription fields. */
export const EVENT_NAME_MAX_LENGTH = INPUT_LIMITS.NAME;
export const EVENT_DESCRIPTION_MAX_LENGTH = 500;

/** A theme palette a tile is drawn in, one per kind of service. */
type TileTone = "primary" | "secondary" | "info" | "success" | "warning" | "error" | "neutral";

/** How a destination looks in lists: a letter tile in its service's tone. */
interface ServiceLook {
  label: string;
  letter: string;
  tone: TileTone;
}

const WEBHOOK_LOOK: ServiceLook = { label: "Webhook", letter: "W", tone: "neutral" };

/**
 * Known services. Letter tiles in theme tones by kind (chat, incident,
 * email, push, SMS) rather than brand colours; a service added later falls
 * back to its own label and a neutral tile.
 */
const SERVICE_LOOKS: Record<string, ServiceLook> = {
  slack: { label: "Slack", letter: "S", tone: "primary" },
  teams: { label: "Microsoft Teams", letter: "T", tone: "primary" },
  googlechat: { label: "Google Chat", letter: "G", tone: "primary" },
  discord: { label: "Discord", letter: "D", tone: "primary" },
  telegram: { label: "Telegram", letter: "T", tone: "primary" },
  mattermost: { label: "Mattermost", letter: "M", tone: "primary" },
  rocketchat: { label: "Rocket.Chat", letter: "R", tone: "primary" },
  zulip: { label: "Zulip", letter: "Z", tone: "primary" },
  pagerduty: { label: "PagerDuty", letter: "P", tone: "error" },
  opsgenie: { label: "Opsgenie", letter: "O", tone: "error" },
  smtp: { label: "Email", letter: "E", tone: "warning" },
  ntfy: { label: "ntfy", letter: "N", tone: "info" },
  gotify: { label: "Gotify", letter: "G", tone: "info" },
  pushover: { label: "Pushover", letter: "P", tone: "info" },
  twilio: { label: "Twilio SMS", letter: "T", tone: "secondary" },
};

/** The services offered first when adding a destination, in this order. */
export const FEATURED_SERVICES = ["slack", "teams", "pagerduty", "googlechat"];

/** Paging services, where sending every event wakes someone up. */
export const PAGING_SERVICES = ["pagerduty", "opsgenie"];

export function lookOf(
  kind: DestinationKind | string,
  service?: string,
  services?: DestinationKindService[],
): ServiceLook {
  if (kind !== "notify") return WEBHOOK_LOOK;
  const known = service ? SERVICE_LOOKS[service] : undefined;
  const offered = services?.find((s) => s.id === service);
  if (known) return { ...known, label: offered?.label ?? known.label };
  const label = offered?.label ?? service ?? "Notification";
  return { label, letter: label.charAt(0).toUpperCase(), tone: "neutral" };
}

/** The letter tile of a destination's service. */
export function DestinationIcon({
  kind,
  service,
  size = 24,
}: {
  kind: DestinationKind | string;
  service?: string;
  size?: number;
}) {
  const look = lookOf(kind, service);
  return (
    <Box
      aria-hidden
      sx={{
        width: size,
        height: size,
        borderRadius: 1,
        bgcolor: look.tone === "neutral" ? "action.selected" : `${look.tone}.main`,
        color: look.tone === "neutral" ? "text.primary" : `${look.tone}.contrastText`,
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        fontSize: size * 0.5,
        fontWeight: 700,
        flexShrink: 0,
        lineHeight: 1,
      }}
    >
      {look.letter}
    </Box>
  );
}

/** A destination's tile and name on one line, as in the subscriptions table. */
export function DestinationName({
  kind,
  service,
  name,
}: {
  kind: DestinationKind | string;
  service?: string;
  name: string;
}) {
  return (
    <Stack direction="row" spacing={1} alignItems="center" sx={{ minWidth: 0 }}>
      <DestinationIcon kind={kind} service={service} size={22} />
      <Typography variant="body2" noWrap title={name}>
        {name}
      </Typography>
    </Stack>
  );
}

/** "All projects", "Org only" or the listed projects. */
export function availabilityLabel(
  availability: DestinationAvailability,
  projects: string[],
): string {
  if (availability === "all") return "All projects";
  if (availability === "org") return "Org only";
  if (projects.length <= 2) return projects.join(", ");
  return `${projects.slice(0, 2).join(", ")} +${projects.length - 2}`;
}

export function HealthChip({ health, failures }: { health: DestinationHealth; failures?: number }) {
  if (health === "healthy") {
    return <Chip size="small" color="success" variant="outlined" label="Healthy" />;
  }
  if (health === "failing") {
    const label = failures && failures > 1 ? `Failing, ${failures} deliveries` : "Failing";
    return <Chip size="small" color="error" variant="outlined" label={label} />;
  }
  return <Chip size="small" variant="outlined" label="No deliveries" />;
}

export function DeliveryStatusChip({
  status,
  attempts,
}: {
  status: EventDeliveryStatus;
  attempts: number;
}) {
  switch (status) {
    case "delivered":
      return <Chip size="small" color="success" variant="outlined" label="Delivered" />;
    case "failed":
      return <Chip size="small" color="error" variant="outlined" label="Failed" />;
    case "skipped":
      return (
        <Tooltip title="No subscription wanted the event any more when it was due to be sent.">
          <Chip size="small" variant="outlined" label="Skipped" />
        </Tooltip>
      );
    default:
      return attempts > 0 ? (
        <Chip size="small" color="warning" variant="outlined" label={`Retrying (${attempts})`} />
      ) : (
        <Chip size="small" color="info" variant="outlined" label="Queued" />
      );
  }
}

/** A subscription's last 24 hours, as in the reference: "6 sent", "1 failed". */
export function Last24hChip({ counts }: { counts: EventDeliveryCounts }) {
  const sent = counts.delivered;
  const failing = counts.failed;
  const title =
    `${sent} delivered, ${counts.failed} failed, ${counts.pending} pending` +
    (counts.skipped ? `, ${counts.skipped} skipped` : "") +
    " in the last 24 hours";
  if (failing > 0) {
    return (
      <Tooltip title={title}>
        <Chip size="small" color="warning" variant="outlined" label={`${failing} failed`} />
      </Tooltip>
    );
  }
  if (sent > 0) {
    return (
      <Tooltip title={title}>
        <Chip size="small" color="success" variant="outlined" label={`${sent} sent`} />
      </Tooltip>
    );
  }
  return (
    <Tooltip title={title}>
      <Typography variant="body2" color="text.secondary">
        {counts.pending > 0 ? `${counts.pending} pending` : "0 sent"}
      </Typography>
    </Tooltip>
  );
}

/** Event types as small monospace chips, "+N" past max. */
export function EventTypeChips({ types, max = 2 }: { types: string[]; max?: number }) {
  if (types.length === 1 && types[0] === ALL_EVENTS) {
    return (
      <Stack direction="row" spacing={1} alignItems="center">
        <Chip size="small" label="*" sx={{ fontFamily: "monospace" }} />
        <Typography variant="body2" color="text.secondary">
          all events
        </Typography>
      </Stack>
    );
  }
  return (
    <Stack direction="row" flexWrap="wrap" alignItems="center" gap={0.75} sx={{ minWidth: 0 }}>
      {types.slice(0, max).map((t) => (
        <Chip key={t} size="small" label={t} sx={{ fontFamily: "monospace", fontSize: 12 }} />
      ))}
      {types.length > max && (
        <Tooltip title={types.slice(max).join(", ")}>
          <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: "nowrap" }}>
            {`+${types.length - max}`}
          </Typography>
        </Tooltip>
      )}
    </Stack>
  );
}

/** "env: production", "all envs". */
export function filterLabel(environments: string[]): string {
  if (environments.length === 0) return "all envs";
  return `env: ${environments.join(", ")}`;
}

export function timeAgo(iso?: string): string {
  return formatRelativeTime(iso, { fallback: "never" });
}

/** A signing secret to show once, with copy, after it is generated. */
export function SecretReveal({
  name,
  secret,
  onClose,
}: {
  name: string;
  secret: string;
  onClose?: () => void;
}) {
  return (
    <Alert
      severity="success"
      onClose={onClose}
      action={
        <Tooltip title="Copy signing secret">
          <IconButton
            size="small"
            aria-label="Copy signing secret"
            onClick={() => navigator.clipboard.writeText(secret)}
          >
            <Copy size={16} />
          </IconButton>
        </Tooltip>
      }
    >
      <Typography variant="body2" fontWeight={600}>
        Signing secret for {name}. Copy it now; it is not shown again.
      </Typography>
      <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
        {secret}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        Requests carry Standard Webhooks headers: verify webhook-signature (HMAC-SHA256 of
        &quot;id.timestamp.body&quot;) with this secret.
      </Typography>
    </Alert>
  );
}

/** The message of a failed request, from the service's error body when it sent one. */
export function errorMessageOf(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const e = error as { message?: unknown; body?: { message?: unknown } };
    if (typeof e.body?.message === "string" && e.body.message) return e.body.message;
    if (typeof e.message === "string" && e.message && !/^HTTP error! status:/i.test(e.message)) {
      return e.message;
    }
  }
  return fallback;
}
