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
import { useSetAgentCardSource } from "@agent-management-platform/api-client";
import { INPUT_LIMITS, type AgentCardPathParams } from "@agent-management-platform/types";
import { validateEndpointUrl } from "@agent-management-platform/shared-component";
import { TextInput } from "@agent-management-platform/views";

interface AgentCardSourceFormProps {
  params: AgentCardPathParams;
  currentUrl?: string;
  /** Compact header editor with tick/X; requires onDone. */
  inline?: boolean;
  /** Called after a successful save or on cancel. */
  onDone?: () => void;
}

const INVALID_HINT = "Enter a public http(s) URL";

/** Card URL for an external A2A agent in one environment. */
export function AgentCardSourceForm({
  params, currentUrl, inline, onDone,
}: AgentCardSourceFormProps) {
  const [url, setUrl] = useState(currentUrl ?? "");
  const { mutate: save, isPending: isSaving } = useSetAgentCardSource();

  const trimmed = url.trim();
  const invalid = trimmed !== "" && validateEndpointUrl(trimmed) !== null;
  const canSave = !!trimmed && !invalid && trimmed !== currentUrl && !isSaving;
  const submit = () => {
    if (canSave) save({ params, body: { url: trimmed } }, { onSuccess: onDone });
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
            if (e.key === "Enter") submit();
            if (e.key === "Escape") onDone?.();
          }}
          error={invalid}
          inputProps={{
            "aria-label": "Agent card URL",
            maxLength: INPUT_LIMITS.URL,
            style: { fontFamily: "monospace" },
          }}
        />
        <Tooltip title={invalid ? INVALID_HINT : "Save"}>
          <span>
            <IconButton
              size="small"
              color="primary"
              aria-label="Save agent card URL"
              disabled={!canSave}
              onClick={submit}
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
        submit();
      }}
      display="flex"
      gap={1}
      alignItems="flex-start"
      sx={{ mb: 1 }}
    >
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
  );
}
