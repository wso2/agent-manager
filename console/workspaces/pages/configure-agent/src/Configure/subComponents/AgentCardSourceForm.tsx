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
import { Box, Button, IconButton, TextField, Tooltip } from "@wso2/oxygen-ui";
import { Check, X } from "@wso2/oxygen-ui-icons-react";
import { useSetA2AProxy, useSetAgentCardSource } from "@agent-management-platform/api-client";
import { INPUT_LIMITS, type AgentCardPathParams } from "@agent-management-platform/types";
import { validateEndpointUrl } from "@agent-management-platform/shared-component";
import { TextInput } from "@agent-management-platform/views";
import { A2AProxyFields } from "./A2AProxyFields";
import { draftFromProxy, KEEP_ORIGIN_HINT, originOf, proxyRequestOf, upstreamAuthError } from "./a2aProxyDraft";

interface AgentCardSourceFormProps {
  params: AgentCardPathParams;
  currentUrl?: string;
  /** Compact header editor with tick/X; requires onDone. */
  inline?: boolean;
  /** Called after a successful save or on cancel. */
  onDone?: () => void;
  /** Offers gateway routing with the new URL; only once the environment is known to be direct. */
  offerRouting?: boolean;
  /** A stored upstream secret pins the card to the origin of currentUrl. */
  keepOrigin?: boolean;
}

const INVALID_HINT = "Enter a public http(s) URL";

// useApiMutation already shows the failure in a snackbar.
const alreadyReported = () => undefined;

/** Card URL for an external A2A agent in one environment. */
export function AgentCardSourceForm({
  params, currentUrl, inline, onDone, offerRouting = false, keepOrigin = false,
}: AgentCardSourceFormProps) {
  const [url, setUrl] = useState(currentUrl ?? "");
  const { mutateAsync: saveSource, isPending: isSaving } = useSetAgentCardSource();
  const { mutateAsync: saveProxy, isPending: isRouting } = useSetA2AProxy();
  const [proxyDraft, setProxyDraft] = useState(() => draftFromProxy(undefined));
  const routes = !inline && offerRouting && proxyDraft.enabled;

  const trimmed = url.trim();
  const proxyError = routes ? upstreamAuthError(proxyDraft, undefined, trimmed) : null;
  const invalid = trimmed !== "" && validateEndpointUrl(trimmed) !== null;
  const leavesOrigin =
    keepOrigin && trimmed !== "" && !invalid && originOf(trimmed) !== originOf(currentUrl ?? "");
  const busy = isSaving || isRouting;
  const canSave =
    !!trimmed && !invalid && !leavesOrigin && trimmed !== currentUrl && !busy && !proxyError;
  // Awaited rather than chained on mutate callbacks: saving the URL unmounts this form.
  const submit = async () => {
    if (!canSave) return;
    const sourceSaved = await saveSource({ params, body: { url: trimmed }, routingFollows: routes })
      .then(() => true, () => false);
    if (!sourceSaved) return;
    onDone?.();
    // The URL stays saved if routing fails; the environment is then left direct.
    if (routes) {
      await saveProxy({ params, body: proxyRequestOf(proxyDraft) }).catch(alreadyReported);
    }
  };

  if (inline) {
    return (
      <Box display="flex" alignItems="center" gap={0.5} sx={{ flex: 1, minWidth: 0 }}>
        <TextField
          size="small"
          autoFocus
          fullWidth
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void submit();
            if (e.key === "Escape") onDone?.();
          }}
          error={invalid}
          inputProps={{
            "aria-label": "Agent card URL",
            maxLength: INPUT_LIMITS.URL,
            style: { fontFamily: "monospace" },
          }}
        />
        <Tooltip title={invalid ? INVALID_HINT : leavesOrigin ? KEEP_ORIGIN_HINT : "Save"}>
          <span>
            <IconButton
              size="small"
              color="primary"
              aria-label="Save agent card URL"
              disabled={!canSave}
              onClick={() => void submit()}
            >
              <Check size={16} />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Cancel">
          <IconButton size="small" aria-label="Cancel" onClick={onDone}>
            <X size={16} />
          </IconButton>
        </Tooltip>
      </Box>
    );
  }

  return (
    <Box
      component="form"
      onSubmit={(e: React.FormEvent) => {
        e.preventDefault();
        void submit();
      }}
      sx={{ mb: 1 }}
    >
      <Box display="flex" gap={1} alignItems="flex-start">
        <TextInput
          label="Agent card URL"
          placeholder="https://agent.example.com/.well-known/agent-card.json"
          value={url}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setUrl(e.target.value)}
          error={invalid}
          helperText={invalid ? INVALID_HINT : undefined}
          maxLength={INPUT_LIMITS.URL}
          fullWidth
        />
        <Button type="submit" variant="contained" size="small" disabled={!canSave} sx={{ mt: 3 }}>
          Save
        </Button>
      </Box>
      {offerRouting && (
        <Box sx={{ mt: 1 }}>
          <A2AProxyFields
            draft={proxyDraft}
            onChange={setProxyDraft}
            hasStoredSecret={false}
            error={proxyError}
            disabled={busy}
          />
        </Box>
      )}
    </Box>
  );
}
