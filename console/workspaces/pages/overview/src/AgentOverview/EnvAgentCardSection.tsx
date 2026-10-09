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

import { useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, IconButton, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Edit, RefreshCw, Trash } from "@wso2/oxygen-ui-icons-react";
import { useDeleteAgentCardSource, useGetAgent, useGetAgentCard, useRefreshAgentCard } from "@agent-management-platform/api-client";
import { CodeBlock, OverviewSectionCard, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { TextInput } from "@agent-management-platform/views";
import type { AgentCardStatus } from "@agent-management-platform/types";
import { AgentCardSourceForm } from "./AgentCardSourceForm";

interface EnvAgentCardSectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
  external?: boolean;
}

const STATUS_CHIP: Record<AgentCardStatus, { label: string; color: "warning" | "success" | "error" }> = {
  pending: { label: "Pending", color: "warning" },
  fetched: { label: "Fetched", color: "success" },
  failed: { label: "Failed", color: "error" },
};

type Json = Record<string, unknown>;

const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const asString = (v: unknown): string | undefined => (typeof v === "string" && v !== "" ? v : undefined);
const objectsIn = (v: unknown): Json[] => (Array.isArray(v) ? v.filter(isObject) : []);
const stringsIn = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : []);

const isNotFound = (error: unknown) => (error as { status?: number } | null)?.status === 404;

/** Stored A2A agent card for the selected environment. */
export function EnvAgentCardSection({
  orgId, projectId, agentId, envId, external,
}: EnvAgentCardSectionProps) {
  const params = { orgName: orgId, projName: projectId, agentName: agentId, envId };
  const { data: agent } = useGetAgent({ orgName: orgId, projName: projectId, agentName: agentId });
  const isA2A = agent?.agentType?.subType === "a2a-agent";
  const { data, isLoading, isError, error } = useGetAgentCard(params, { enabled: isA2A });
  const { mutate: refresh, isPending: isRefreshing } = useRefreshAgentCard();
  const { mutate: removeSource, isPending: isRemoving } = useDeleteAgentCardSource();
  const { addConfirmation } = useConfirmationDialog();
  const [showJson, setShowJson] = useState(false);
  const [editingUrl, setEditingUrl] = useState(false);

  if (!isA2A) {
    return null;
  }
  const noSource = isError && isNotFound(error);
  // A 404 leaves the previous card cached; it no longer exists.
  const card = noSource ? undefined : data;
  // Card content is third-party: read it as untyped JSON.
  const cardJson: Json = isObject(card?.card) ? card.card : {};
  const cardName = asString(cardJson.name);
  const cardDescription = asString(cardJson.description);
  const version = asString(cardJson.version)?.replace(/^v/i, "");
  const sourceUrl = card?.sourceUrl;
  const showUrlForm = external && noSource;

  const confirmRemove = () => addConfirmation({
    analytics: { entity: "agent-card-source", action: "remove" },
    title: "Remove agent card URL",
    description: "The stored agent card for this environment will be removed. You can register a URL again later.",
    confirmButtonText: "Remove",
    confirmButtonColor: "error",
    confirmButtonIcon: <Trash size={16} />,
    onConfirm: () => removeSource(params),
  });

  return (
    <OverviewSectionCard
      title="Agent Card"
      titleAdornment={
        sourceUrl && (editingUrl ? (
          <AgentCardSourceForm
            inline
            params={params}
            currentUrl={sourceUrl}
            onDone={() => setEditingUrl(false)}
          />
        ) : (
          <>
            <Typography variant="body2" color="text.secondary" noWrap sx={{ fontFamily: "monospace" }}>
              {sourceUrl}
            </Typography>
            {external && (
              <Tooltip title="Edit URL">
                <IconButton size="small" onClick={() => setEditingUrl(true)} sx={{ p: 0.25, flexShrink: 0 }}>
                  <Edit size={14} />
                </IconButton>
              </Tooltip>
            )}
          </>
        ))
      }
      headerAction={
        !(noSource && external) && (
          <Button
            size="small"
            variant="text"
            startIcon={isRefreshing ? <CircularProgress size={14} /> : <RefreshCw size={14} />}
            disabled={isRefreshing}
            onClick={() => refresh(params)}
          >
            Refetch
          </Button>
        )
      }
      sx={{ mb: 1.5 }}
    >
      {showUrlForm && (
        <AgentCardSourceForm params={params} currentUrl={sourceUrl} />
      )}
      {isLoading && <CircularProgress size={16} />}
      {noSource && (
        <Typography variant="body2" color="text.secondary">
          {external
            ? "Register this agent's card URL for the environment to fetch its card."
            : "No agent card has been fetched for this environment yet."}
        </Typography>
      )}
      {isError && !noSource && (
        <Typography variant="body2" color="error">Unable to load the agent card. Try again later.</Typography>
      )}
      {card && (
        <>
          <Box display="flex" alignItems="center" gap={1} sx={{ mb: 1 }}>
            {card.status !== "fetched" && (
              <Chip
                variant="outlined"
                size="small"
                label={STATUS_CHIP[card.status].label}
                color={STATUS_CHIP[card.status].color}
              />
            )}
            {card.fetchedAt && (
              <Chip
                variant="outlined"
                size="small"
                label={`Fetched ${new Date(card.fetchedAt).toLocaleString()}`}
              />
            )}
          </Box>
          {card.status === "failed" && card.lastError && (
            <Alert severity="error" sx={{ mb: 1 }}>Fetch failed: {card.lastError}</Alert>
          )}
          {card.card && (
            <Stack spacing={3} sx={{ mt: 2 }}>
              <Box>
                <Box display="flex" alignItems="baseline" gap={1}>
                  <Typography variant="h6">{cardName}</Typography>
                  {version && (
                    <Typography variant="caption" color="text.secondary">v{version}</Typography>
                  )}
                </Box>
                {cardDescription && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{cardDescription}</Typography>
                )}
              </Box>
              <Box>
                <Typography variant="overline" color="text.secondary" display="block" sx={{ mb: 1 }}>Interfaces</Typography>
                <Stack spacing={1}>
                  {objectsIn(cardJson.supportedInterfaces).map((iface, i) => {
                    const url = asString(iface.url);
                    const binding = asString(iface.protocolBinding);
                    return (
                      <Box key={`${i}-${url ?? ""}`} display="flex" gap={1.5} alignItems="center">
                        {binding && <Chip size="small" label={binding} sx={{ minWidth: 96 }} />}
                        <TextInput
                          value={url ?? ""}
                          copyable
                          copyTooltipText="Copy URL"
                          size="small"
                          slotProps={{ input: { readOnly: true, sx: { fontFamily: "monospace" } } }}
                        />
                      </Box>
                    );
                  })}
                </Stack>
              </Box>
              <Box>
                <Typography variant="overline" color="text.secondary" display="block" sx={{ mb: 1 }}>Skills</Typography>
                <Stack spacing={1.5}>
                  {objectsIn(cardJson.skills).map((skill, i) => {
                    const name = asString(skill.name);
                    const description = asString(skill.description);
                    const tags = stringsIn(skill.tags);
                    return (
                      <Box
                        key={`${i}-${asString(skill.id) ?? name ?? ""}`}
                        sx={{ p: 2, border: 1, borderColor: "divider", borderRadius: 1 }}
                      >
                        <Typography variant="subtitle2">{name}</Typography>
                        {description && (
                          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{description}</Typography>
                        )}
                        {tags.length > 0 && (
                          <Box display="flex" gap={0.75} flexWrap="wrap" sx={{ mt: 1.5 }}>
                            {tags.map((tag, j) => <Chip key={`${j}-${tag}`} size="small" variant="outlined" label={tag} />)}
                          </Box>
                        )}
                      </Box>
                    );
                  })}
                </Stack>
              </Box>
              <Box>
                <Button size="small" variant="text" onClick={() => setShowJson((v) => !v)} sx={{ ml: -1 }}>
                  {showJson ? "Hide JSON" : "View JSON"}
                </Button>
                {showJson && (
                  <Box sx={{ mt: 1 }}>
                    <CodeBlock code={JSON.stringify(card.card, null, 2)} language="json" fieldId="agent-card-json" analyticsId="agent-card-json" />
                  </Box>
                )}
              </Box>
            </Stack>
          )}
        </>
      )}
      {external && sourceUrl && (
        <Box display="flex" justifyContent="flex-end" sx={{ mt: 3 }}>
          <Button
            size="small"
            variant="text"
            color="error"
            startIcon={<Trash size={14} />}
            disabled={isRemoving}
            onClick={confirmRemove}
          >
            Remove
          </Button>
        </Box>
      )}
    </OverviewSectionCard>
  );
}
