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
  Alert,
  Avatar,
  Box,
  Button,
  CardContent,
  CardHeader,
  Chip,
  Form,
  FormControlLabel,
  Skeleton,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Check, CircleIcon } from "@wso2/oxygen-ui-icons-react";
import { useWebhookEventTypes } from "@agent-management-platform/api-client";
import type {
  WebhookEventType,
  WebhookRequest,
  WebhookResponse,
  WebhookScope,
} from "@agent-management-platform/types";

const NAME_MAX_LENGTH = 100;
const DESCRIPTION_MAX_LENGTH = 500;

function isValidUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

export interface EnvironmentOption {
  name: string;
  label: string;
}

export interface WebhookFormProps {
  orgName?: string;
  scope: WebhookScope;
  /** The webhook being edited; undefined to create one. */
  webhook?: WebhookResponse;
  /** Environments an agent webhook selects from; undefined for other scopes. */
  environmentOptions?: EnvironmentOption[];
  /** Environments preselected for a new agent webhook. */
  defaultEnvironments?: string[];
  saving: boolean;
  submitLabel: string;
  onCancel: () => void;
  onSubmit: (body: WebhookRequest) => void;
}

/** Selection marker used on every selectable card, as on the evaluator cards. */
function SelectionAvatar({ selected }: { selected: boolean }) {
  return (
    <Avatar
      sx={{
        bgcolor: selected ? "primary.main" : "default",
        color: selected ? "primary.contrastText" : "text.secondary",
        width: 32,
        height: 32,
        flexShrink: 0,
      }}
    >
      {selected ? <Check size={16} /> : <CircleIcon size={16} />}
    </Avatar>
  );
}

const cardGrid = {
  display: "grid",
  gridTemplateColumns: {
    xs: "repeat(auto-fill, minmax(260px, 1fr))",
    md: "repeat(auto-fill, minmax(300px, 1fr))",
  },
  gap: 2,
} as const;

/** Create or edit a webhook, laid out like the monitor creation form. */
export function WebhookForm({
  orgName,
  scope,
  webhook,
  environmentOptions,
  defaultEnvironments,
  saving,
  submitLabel,
  onCancel,
  onSubmit,
}: WebhookFormProps) {
  const { data: catalog, isLoading } = useWebhookEventTypes(orgName, scope);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [url, setUrl] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [envs, setEnvs] = useState<Set<string>>(new Set());
  const [submitted, setSubmitted] = useState(false);
  const isAgent = !!environmentOptions;

  useEffect(() => {
    setName(webhook?.name ?? "");
    setDescription(webhook?.description ?? "");
    setUrl(webhook?.url ?? "");
    setEnabled(webhook?.enabled ?? true);
    setSelected(new Set(webhook?.eventTypes ?? []));
    setEnvs(new Set(webhook?.environments ?? defaultEnvironments ?? []));
    setSubmitted(false);
  }, [webhook, defaultEnvironments]);

  const groups = useMemo(() => {
    const byCategory = new Map<string, WebhookEventType[]>();
    for (const t of catalog?.eventTypes ?? []) {
      byCategory.set(t.category, [...(byCategory.get(t.category) ?? []), t]);
    }
    return [...byCategory.entries()];
  }, [catalog]);

  const offered = useMemo(
    () => new Set((catalog?.eventTypes ?? []).map((t) => t.type)),
    [catalog],
  );

  const toggleSet = (setter: typeof setSelected, values: string[], on: boolean) =>
    setter((current) => {
      const next = new Set(current);
      values.forEach((v) => (on ? next.add(v) : next.delete(v)));
      return next;
    });

  const nameError = submitted && !name.trim() ? "Enter a name" : undefined;
  const urlError = submitted && !isValidUrl(url.trim()) ? "Enter an http(s) URL" : undefined;
  const envError =
    submitted && isAgent && envs.size === 0 ? "Select at least one environment" : undefined;

  const handleSubmit = () => {
    setSubmitted(true);
    if (!name.trim() || !isValidUrl(url.trim())) return;
    if (isAgent && envs.size === 0) return;
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      url: url.trim(),
      environments: isAgent ? [...envs] : undefined,
      enabled,
      // Keep only types this scope still offers, so a webhook saved before an
      // event moved to another scope can be edited without an error.
      eventTypes: [...selected].filter((t) => offered.has(t)),
    });
  };

  return (
    <Stack spacing={3}>
      <Form.Stack>
        <Form.Section>
          <Form.Header>Basic Details</Form.Header>
          <Form.ElementWrapper name="name" label="Name">
            <TextField
              id="name"
              required
              fullWidth
              placeholder="Enter webhook name"
              slotProps={{ htmlInput: { maxLength: NAME_MAX_LENGTH } }}
              value={name}
              onChange={(e) => setName(e.target.value)}
              error={!!nameError}
              helperText={nameError ?? "Visible label shown in the webhooks list"}
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper name="description" label="Description">
            <TextField
              id="description"
              fullWidth
              multiline
              minRows={3}
              placeholder="Enter webhook description"
              slotProps={{ htmlInput: { maxLength: DESCRIPTION_MAX_LENGTH } }}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper name="url" label="Endpoint URL">
            <TextField
              id="url"
              required
              fullWidth
              placeholder="https://hooks.example.com/amp"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              error={!!urlError}
              helperText={
                urlError ?? "Events are sent here as signed JSON POST requests"
              }
            />
          </Form.ElementWrapper>
          <FormControlLabel
            control={<Switch checked={enabled} onChange={(_, v) => setEnabled(v)} />}
            label="Send events to this webhook"
          />
        </Form.Section>

        {isAgent && (
          <Form.Section>
            <Form.Header>Environments</Form.Header>
            <Typography variant="caption" color="text.secondary">
              Events in the selected environments are sent to this webhook.
              Agent events that are not tied to an environment, such as builds,
              are always sent.
            </Typography>
            <Box sx={cardGrid}>
              {environmentOptions.map((env) => {
                const isSelected = envs.has(env.name);
                return (
                  <Form.CardButton
                    key={env.name}
                    selected={isSelected}
                    sx={{ width: "100%", minWidth: 0, justifyContent: "flex-start" }}
                    onClick={() => toggleSet(setEnvs, [env.name], !isSelected)}
                  >
                    <CardHeader
                      title={
                        <Stack direction="row" spacing={2} alignItems="center">
                          <SelectionAvatar selected={isSelected} />
                          <Typography variant="h6">{env.label}</Typography>
                        </Stack>
                      }
                    />
                  </Form.CardButton>
                );
              })}
            </Box>
            {envError && (
              <Typography variant="caption" color="error">
                {envError}
              </Typography>
            )}
          </Form.Section>
        )}

        <Form.Section>
          <Form.Header>Events</Form.Header>
          <Typography variant="caption" color="text.secondary">
            Choose the events to send. Nothing is sent until you select at least one.
          </Typography>
          {isLoading ? (
            <Box sx={cardGrid}>
              <Skeleton variant="rounded" height={120} />
              <Skeleton variant="rounded" height={120} />
              <Skeleton variant="rounded" height={120} />
            </Box>
          ) : (
            <Stack spacing={3}>
              {groups.map(([category, types]) => {
                const names = types.map((t) => t.type);
                const count = names.filter((n) => selected.has(n)).length;
                const all = count === names.length;
                return (
                  <Stack key={category} spacing={1.5}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Typography variant="subtitle1" fontWeight={600}>
                        {category}
                      </Typography>
                      <Chip
                        size="small"
                        variant="outlined"
                        label={`${count}/${names.length}`}
                        color={count > 0 ? "primary" : "default"}
                      />
                      <Button
                        size="small"
                        variant="text"
                        onClick={() => toggleSet(setSelected, names, !all)}
                      >
                        {all ? "Clear" : "Select all"}
                      </Button>
                    </Stack>
                    <Box sx={cardGrid}>
                      {types.map((t) => {
                        const isSelected = selected.has(t.type);
                        return (
                          <Form.CardButton
                            key={t.type}
                            selected={isSelected}
                            sx={{
                              width: "100%",
                              minWidth: 0,
                              justifyContent: "flex-start",
                              overflow: "hidden",
                            }}
                            onClick={() => toggleSet(setSelected, [t.type], !isSelected)}
                          >
                            <CardHeader
                              sx={{
                                width: "100%",
                                minWidth: 0,
                                "& .MuiCardHeader-content": { overflow: "hidden", minWidth: 0 },
                              }}
                              title={
                                <Stack direction="row" spacing={2} alignItems="center">
                                  <SelectionAvatar selected={isSelected} />
                                  <Typography
                                    variant="body1"
                                    fontWeight={600}
                                    noWrap
                                    sx={{ fontFamily: "monospace", minWidth: 0 }}
                                  >
                                    {t.type}
                                  </Typography>
                                </Stack>
                              }
                            />
                            <CardContent sx={{ pt: 0 }}>
                              <Typography variant="caption">{t.description}</Typography>
                            </CardContent>
                          </Form.CardButton>
                        );
                      })}
                    </Box>
                  </Stack>
                );
              })}
            </Stack>
          )}
        </Form.Section>
      </Form.Stack>

      {submitted && selected.size === 0 && (
        <Alert severity="info">
          No events are selected, so this webhook will not receive anything yet.
        </Alert>
      )}

      <Stack direction="row" gap={2}>
        <Button variant="outlined" color="primary" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button variant="contained" color="primary" onClick={handleSubmit} disabled={saving}>
          {submitLabel}
        </Button>
      </Stack>
    </Stack>
  );
}

export default WebhookForm;
