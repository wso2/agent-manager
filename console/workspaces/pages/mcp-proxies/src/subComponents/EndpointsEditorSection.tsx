/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useMemo } from "react";
import { Box, Button, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import type { Environment } from "@agent-management-platform/types";
import { EndpointFormFields, type EndpointDraft } from "./EndpointFormFields";
import { EndpointRow } from "./EndpointRow";

// Backend endpoint ids are handles derived from a user-chosen display name
// (lowercased, alphanumeric + hyphen — see GenerateHandle server-side), so a
// simple counter like "1" can collide with a real endpoint's id. A UUID can't
// collide in practice, keeping new drafts from being mistaken for — and
// merged with — an existing endpoint's saved config on submit.
function createDraftEndpointId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export interface EndpointsEditorSectionProps {
  orgId: string;
  environments: Environment[];
  endpoints: EndpointDraft[];
  onEndpointsChange: (endpoints: EndpointDraft[]) => void;
  addOpen: boolean;
  onAddOpenChange: (open: boolean) => void;
  editingId: string | null;
  onEditingIdChange: (id: string | null) => void;
  emptyStateText: string;
  // Fires once, right after a brand-new endpoint is added (not on edits) — used by
  // the create-proxy form to seed the proxy name/version from the fetched server.
  onEndpointAdded?: (draft: Omit<EndpointDraft, "id">) => void;
  // Whether the open add/edit form holds edits not yet applied to `endpoints`.
  onEndpointFormDirtyChange?: (dirty: boolean) => void;
}

// Shared by AddMCPProxyForm and EditMCPProxy: renders the endpoint list with
// add-below-list / edit-in-place inline forms, guarding so only one form (add or
// edit) is open at a time. `endpoints`/`addOpen`/`editingId` are controlled by the
// parent since it also needs them to build its submit payload and to hide its own
// page-level actions while a form is open.
export function EndpointsEditorSection({
  orgId,
  environments,
  endpoints,
  onEndpointsChange,
  addOpen,
  onAddOpenChange,
  editingId,
  onEditingIdChange,
  emptyStateText,
  onEndpointAdded,
  onEndpointFormDirtyChange,
}: EndpointsEditorSectionProps) {
  const environmentLabels = useMemo(() => {
    const labels = new Map<string, string>();
    environments.forEach((env) => {
      if (env.id) labels.set(env.id, env.displayName || env.name);
    });
    return labels;
  }, [environments]);

  const usedEnvIds = useMemo(() => {
    const used = new Set<string>();
    endpoints.forEach((endpoint) => {
      endpoint.environments.forEach((envId) => used.add(envId));
    });
    return used;
  }, [endpoints]);

  const editingEndpoint = useMemo(
    () => endpoints.find((endpoint) => endpoint.id === editingId),
    [endpoints, editingId],
  );

  // Environments selectable in the endpoint form. When editing, the endpoint's own
  // environments stay available; only those claimed by other endpoints are excluded.
  const availableEnvironments = useMemo(() => {
    const ownEnvIds = new Set(editingEndpoint?.environments ?? []);
    return environments.filter(
      (env) => !!env.id && (ownEnvIds.has(env.id) || !usedEnvIds.has(env.id)),
    );
  }, [environments, usedEnvIds, editingEndpoint]);

  const closeEndpointForm = useCallback(() => {
    onAddOpenChange(false);
    onEditingIdChange(null);
  }, [onAddOpenChange, onEditingIdChange]);

  const handleAddEndpoint = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      onEndpointsChange([...endpoints, { ...draft, id: createDraftEndpointId() }]);
      onEndpointAdded?.(draft);
      onAddOpenChange(false);
    },
    [endpoints, onEndpointsChange, onEndpointAdded, onAddOpenChange],
  );

  const handleSaveEditedEndpoint = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      onEndpointsChange(
        endpoints.map((endpoint) =>
          endpoint.id === editingId ? { ...draft, id: endpoint.id } : endpoint,
        ),
      );
      onEditingIdChange(null);
    },
    [endpoints, editingId, onEndpointsChange, onEditingIdChange],
  );

  const handleRemoveEndpoint = useCallback(
    (id: string) => {
      onEndpointsChange(endpoints.filter((endpoint) => endpoint.id !== id));
    },
    [endpoints, onEndpointsChange],
  );

  const noEnvironmentsLeft =
    environments.length > 0 && availableEnvironments.length === 0;

  return (
    <>
      {endpoints.length > 0 || addOpen ? (
        <Stack spacing={1.5}>
          {endpoints.map((endpoint) =>
            editingId === endpoint.id ? (
              <Box
                key={endpoint.id}
                sx={{
                  border: "1px solid",
                  borderColor: "primary.main",
                  borderRadius: 1,
                  p: 2,
                }}
              >
                <EndpointFormFields
                  orgId={orgId}
                  availableEnvironments={availableEnvironments}
                  initialDraft={editingEndpoint}
                  onAdd={handleSaveEditedEndpoint}
                  onCancel={closeEndpointForm}
                  onDirtyChange={onEndpointFormDirtyChange}
                />
              </Box>
            ) : (
              <EndpointRow
                key={endpoint.id}
                endpoint={endpoint}
                environmentLabels={environmentLabels}
                // Only one endpoint form (add or edit) is open at a time, so
                // hide every other row's Edit action while one is in progress
                // rather than letting a second form open on top of it.
                onEdit={
                  addOpen || editingId !== null
                    ? undefined
                    : () => onEditingIdChange(endpoint.id)
                }
                onRemove={() => handleRemoveEndpoint(endpoint.id)}
              />
            ),
          )}

          {addOpen && (
            <Box
              sx={{
                border: "1px solid",
                borderColor: "primary.main",
                borderRadius: 1,
                p: 2,
              }}
            >
              <EndpointFormFields
                orgId={orgId}
                availableEnvironments={availableEnvironments}
                onAdd={handleAddEndpoint}
                onCancel={closeEndpointForm}
                onDirtyChange={onEndpointFormDirtyChange}
              />
            </Box>
          )}
        </Stack>
      ) : (
        <Box
          sx={{
            border: "1px dashed",
            borderColor: "divider",
            borderRadius: 1,
            px: 2,
            py: 3,
            textAlign: "center",
          }}
        >
          <Typography variant="body2" color="text.secondary">
            {emptyStateText}
          </Typography>
        </Box>
      )}

      {!addOpen && editingId === null && (
        <Box>
          <Tooltip
            title={
              noEnvironmentsLeft
                ? "All environments already have an endpoint."
                : ""
            }
            disableHoverListener={!noEnvironmentsLeft}
          >
            <span>
              <Button
                variant="outlined"
                startIcon={<Plus size={16} />}
                onClick={() => onAddOpenChange(true)}
                disabled={noEnvironmentsLeft || environments.length === 0}
              >
                Add Endpoint
              </Button>
            </span>
          </Tooltip>
        </Box>
      )}

      {environments.length > 1 ? (
        <Typography variant="caption" color="text.secondary">
          {usedEnvIds.size} of {environments.length} environments have an
          endpoint.
        </Typography>
      ) : null}
    </>
  );
}

export default EndpointsEditorSection;
