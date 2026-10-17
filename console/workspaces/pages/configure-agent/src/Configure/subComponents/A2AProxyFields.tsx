/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Box, Button, FormControlLabel, Stack, Switch, Typography } from "@wso2/oxygen-ui";
import { TextInput } from "@agent-management-platform/views";
import { type A2AProxyDraft, UPSTREAM_HEADER_MAX, UPSTREAM_VALUE_MAX_BYTES } from "./a2aProxyDraft";

interface A2AProxyFieldsProps {
  draft: A2AProxyDraft;
  onChange: (draft: A2AProxyDraft) => void;
  /** True when the server stores a secret for the saved header. */
  hasStoredSecret: boolean;
  error: string | null;
  disabled?: boolean;
}

/** The route-through-gateway switch and an optional upstream header; the secret is write-only. */
export function A2AProxyFields({
  draft, onChange, hasStoredSecret, error, disabled,
}: A2AProxyFieldsProps) {
  const set = (patch: Partial<A2AProxyDraft>) => onChange({ ...draft, ...patch });
  return (
    <Stack spacing={1.5}>
      <FormControlLabel
        control={
          <Switch
            checked={draft.enabled}
            disabled={disabled}
            onChange={(_, enabled) => set({ enabled })}
          />
        }
        label="Route through gateway"
      />
      {draft.enabled && (
        <>
          <Typography variant="caption" color="text.secondary">
            Callers reach the agent through this environment&apos;s gateway, with its API keys,
            CORS and observability. Add a header if the agent itself requires a credential.
          </Typography>
          <TextInput
            size="small"
            label="Upstream header"
            placeholder="X-API-Key"
            value={draft.header}
            disabled={disabled}
            onChange={(e) => set({ header: e.target.value })}
            maxLength={UPSTREAM_HEADER_MAX}
          />
          {hasStoredSecret && draft.secret === "keep" && (
            <Box display="flex" alignItems="center" gap={1}>
              <Typography variant="body2" color="text.secondary">A value is stored.</Typography>
              <Button size="small" disabled={disabled} onClick={() => set({ secret: "replace", value: "" })}>Replace</Button>
              <Button size="small" color="error" disabled={disabled} onClick={() => set({ secret: "clear" })}>Clear</Button>
            </Box>
          )}
          {hasStoredSecret && draft.secret === "clear" && (
            <Box display="flex" alignItems="center" gap={1}>
              <Typography variant="body2" color="text.secondary">The stored value is removed when you save.</Typography>
              <Button size="small" disabled={disabled} onClick={() => set({ secret: "keep" })}>Undo</Button>
            </Box>
          )}
          {draft.secret === "replace" && (
            <Box display="flex" alignItems="center" gap={1}>
              <TextInput
                size="small"
                type="password"
                showPasswordToggle
                autoComplete="new-password"
                label="Header value"
                value={draft.value}
                disabled={disabled}
                onChange={(e) => set({ value: e.target.value })}
                // Chars never exceed bytes, so this cap never blocks a valid value.
                maxLength={UPSTREAM_VALUE_MAX_BYTES}
              />
              {hasStoredSecret && (
                <Button
                  size="small"
                  disabled={disabled}
                  onClick={() => set({ secret: "keep", value: "" })}
                >
                  Keep stored value
                </Button>
              )}
            </Box>
          )}
          {error && <Typography variant="caption" color="error">{error}</Typography>}
        </>
      )}
    </Stack>
  );
}
