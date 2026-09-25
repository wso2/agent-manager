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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  type MCPEndpointConfig,
  type MCPProxy,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChevronDown, HelpCircle } from "@wso2/oxygen-ui-icons-react";
import {
  ResilienceTimeoutFields,
  validateEndpointUrl,
} from "@agent-management-platform/shared-component";
import { AuthHeaderRow } from "./AuthHeaderRow";

const MASKED_CREDENTIAL_VALUE = "••••••••••••";
const DURATION_PATTERN = /^\d+(\.\d+)?(ms|s|m|h)$/;

export type MCPProxyConnectionTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxyConnectionTab({
  config,
  selectedEndpointId,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxyConnectionTabProps) {
  const theme = useTheme();
  const initializedEnvIdRef = useRef<string | null>(null);

  const [endpoint, setEndpoint] = useState("");
  const [authHeader, setAuthHeader] = useState("");
  // Mirrors Postman's per-header checkbox: unchecking excludes the header from
  // save without losing the typed key/value.
  const [authEnabled, setAuthEnabled] = useState(true);
  const [credentialValue, setCredentialValue] = useState("");
  const [isCredentialMasked, setIsCredentialMasked] = useState(false);
  const [showCredential, setShowCredential] = useState(false);
  const [resilienceTimeout, setResilienceTimeout] = useState("");
  const [resilienceIdleTimeout, setResilienceIdleTimeout] = useState("");
  const [endpointError, setEndpointError] = useState<string | null>(null);
  const [resilienceTimeoutError, setResilienceTimeoutError] = useState<string | null>(
    null,
  );
  const [resilienceIdleTimeoutError, setResilienceIdleTimeoutError] = useState<
    string | null
  >(null);
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);

  // Header and credential are stored together, so an existing header implies a
  // stored credential (the value itself is never returned by the backend).
  const hasStoredCredential = useMemo(
    () => Boolean(config?.upstream?.main?.auth?.header),
    [config?.upstream?.main?.auth?.header],
  );

  const resetFromConfig = useCallback(() => {
    setEndpoint(config?.upstream?.main?.url ?? "");
    setAuthHeader(config?.upstream?.main?.auth?.header ?? "");
    setAuthEnabled(true);
    const hasCredential = Boolean(config?.upstream?.main?.auth?.header);
    setCredentialValue(hasCredential ? MASKED_CREDENTIAL_VALUE : "");
    setIsCredentialMasked(hasCredential);
    setShowCredential(false);
    setResilienceTimeout(config?.resilience?.timeout ?? "");
    setResilienceIdleTimeout(config?.resilience?.idleTimeout ?? "");
    setEndpointError(null);
    setResilienceTimeoutError(null);
    setResilienceIdleTimeoutError(null);
  }, [config]);

  useEffect(() => {
    if (!selectedEndpointId) return;
    if (initializedEnvIdRef.current === selectedEndpointId) return;
    initializedEnvIdRef.current = selectedEndpointId;
    resetFromConfig();
  }, [selectedEndpointId, resetFromConfig]);

  const validateEndpoint = useCallback((value: string): string | null => {
    const err = validateEndpointUrl(value, {
      requiredMessage: "MCP Server Endpoint URL is required",
    });
    setEndpointError(err);
    return err;
  }, []);

  const validateResilienceTimeout = useCallback((value: string): string | null => {
    const trimmed = value.trim();
    const err =
      trimmed && !DURATION_PATTERN.test(trimmed)
        ? "Enter a duration like 5s, 500ms, or 1m"
        : null;
    setResilienceTimeoutError(err);
    return err;
  }, []);

  const validateResilienceIdleTimeout = useCallback((value: string): string | null => {
    const trimmed = value.trim();
    const err =
      trimmed && !DURATION_PATTERN.test(trimmed)
        ? "Enter a duration like 5s, 500ms, or 1m"
        : null;
    setResilienceIdleTimeoutError(err);
    return err;
  }, []);

  const credentialChanged =
    authEnabled &&
    !isCredentialMasked &&
    credentialValue.trim() !== MASKED_CREDENTIAL_VALUE;
  const effectiveAuthHeader = authEnabled ? authHeader.trim() : "";

  const isDirty = useMemo(() => {
    if (!config) return false;
    const savedUrl = (config.upstream?.main?.url ?? "").trim();
    const savedHeader = (config.upstream?.main?.auth?.header ?? "").trim();
    const savedResilienceTimeout = (config.resilience?.timeout ?? "").trim();
    const savedResilienceIdleTimeout = (config.resilience?.idleTimeout ?? "").trim();
    if (endpoint.trim() !== savedUrl) return true;
    if (effectiveAuthHeader !== savedHeader) return true;
    if (resilienceTimeout.trim() !== savedResilienceTimeout) return true;
    if (resilienceIdleTimeout.trim() !== savedResilienceIdleTimeout) return true;
    if (credentialChanged) return true;
    return false;
  }, [
    config,
    endpoint,
    effectiveAuthHeader,
    resilienceTimeout,
    resilienceIdleTimeout,
    credentialChanged,
  ]);

  const handleDiscard = useCallback(() => {
    resetFromConfig();
    setStatus(null);
  }, [resetFromConfig]);

  const handleSave = useCallback(async () => {
    if (!config) return;

    const endpointValidationError = validateEndpoint(endpoint);
    if (endpointValidationError) {
      setStatus({ message: endpointValidationError, severity: "error" });
      return;
    }

    const resilienceTimeoutValidationError = validateResilienceTimeout(resilienceTimeout);
    if (resilienceTimeoutValidationError) {
      setStatus({ message: resilienceTimeoutValidationError, severity: "error" });
      return;
    }

    const resilienceIdleTimeoutValidationError =
      validateResilienceIdleTimeout(resilienceIdleTimeout);
    if (resilienceIdleTimeoutValidationError) {
      setStatus({ message: resilienceIdleTimeoutValidationError, severity: "error" });
      return;
    }
    const existingAuth = config.upstream?.main?.auth;
    // Preserve any existing auth (including its type); only override header,
    // and value when the user typed a new one — otherwise the backend keeps
    // the stored credential. Unchecking the header excludes it entirely.
    const auth = effectiveAuthHeader
      ? {
          type: "api-key" as const,
          ...existingAuth,
          header: effectiveAuthHeader,
          ...(credentialChanged ? { value: credentialValue.trim() } : {}),
        }
      : undefined;

    const trimmedResilienceTimeout = resilienceTimeout.trim();
    const trimmedResilienceIdleTimeout = resilienceIdleTimeout.trim();

    try {
      await onUpdate({
        upstream: {
          ...config.upstream,
          main: {
            ...config.upstream?.main,
            url: endpoint.trim(),
            auth,
          },
        },
        resilience:
          trimmedResilienceTimeout || trimmedResilienceIdleTimeout
            ? {
                timeout: trimmedResilienceTimeout || undefined,
                idleTimeout: trimmedResilienceIdleTimeout || undefined,
              }
            : undefined,
      });
      setStatus({
        message: "Connection updated successfully.",
        severity: "success",
      });
      if (credentialChanged) {
        setCredentialValue(MASKED_CREDENTIAL_VALUE);
        setIsCredentialMasked(true);
      }
    } catch {
      setStatus({ message: "Failed to update connection.", severity: "error" });
    }
  }, [
    config,
    endpoint,
    effectiveAuthHeader,
    resilienceTimeout,
    resilienceIdleTimeout,
    credentialChanged,
    credentialValue,
    onUpdate,
    validateEndpoint,
    validateResilienceTimeout,
    validateResilienceIdleTimeout,
  ]);

  if (isLoading) {
    return (
      <Stack spacing={2}>
        {[1, 2, 3].map((i) => (
          <Stack key={i} spacing={0.5}>
            <Skeleton variant="text" width={160} height={16} />
            <Skeleton variant="rounded" height={40} />
          </Stack>
        ))}
      </Stack>
    );
  }

  if (!config) {
    return null;
  }

  return (
    <Stack spacing={2}>
      <Grid container spacing={3}>
        <Grid size={{ xs: 12 }}>
          <FormControl fullWidth>
            <FormLabel required>MCP Server Endpoint URL</FormLabel>
            <TextField
              slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
              size="small"
              value={endpoint}
              onChange={(e) => {
                setEndpoint(e.target.value);
                if (endpointError) validateEndpoint(e.target.value);
              }}
              onBlur={() => validateEndpoint(endpoint)}
              error={!!endpointError}
              helperText={endpointError}
              placeholder="Enter URL of your MCP Server"
              sx={{
                "& .MuiInputBase-input": {
                  fontFamily: "monospace",
                  fontSize: theme.typography.body2?.fontSize,
                },
              }}
            />
          </FormControl>
        </Grid>

        <Grid size={{ xs: 12 }}>
          <Accordion defaultExpanded disableGutters variant="outlined">
            <AccordionSummary expandIcon={<ChevronDown size={18} />}>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Typography variant="subtitle2" fontWeight={600}>
                  Advanced Configurations
                </Typography>
                <Tooltip title="Configure an optional authentication header sent to the MCP Server endpoint.">
                  <HelpCircle size={16} />
                </Tooltip>
              </Stack>
            </AccordionSummary>
            <AccordionDetails>
              <AuthHeaderRow
                enabled={authEnabled}
                onEnabledChange={setAuthEnabled}
                headerValue={authHeader}
                onHeaderChange={setAuthHeader}
                valueValue={credentialValue}
                onValueFocus={() => {
                  if (isCredentialMasked) {
                    setCredentialValue("");
                    setIsCredentialMasked(false);
                  }
                }}
                onValueChange={setCredentialValue}
                showValue={showCredential}
                onToggleShowValue={() => setShowCredential((p) => !p)}
                caption={
                  hasStoredCredential
                    ? "The stored value is hidden. Leave it unchanged to keep the current credential, or enter a new value to replace it."
                    : null
                }
                monospaceValue
                iconButtonSize="small"
              />
            </AccordionDetails>
          </Accordion>
        </Grid>

        <Grid size={{ xs: 12 }}>
          <ResilienceTimeoutFields
            requestTimeout={resilienceTimeout}
            onRequestTimeoutChange={(value) => {
              setResilienceTimeout(value);
              if (resilienceTimeoutError) validateResilienceTimeout(value);
            }}
            onRequestTimeoutBlur={() => validateResilienceTimeout(resilienceTimeout)}
            requestTimeoutError={resilienceTimeoutError}
            idleTimeout={resilienceIdleTimeout}
            onIdleTimeoutChange={(value) => {
              setResilienceIdleTimeout(value);
              if (resilienceIdleTimeoutError) validateResilienceIdleTimeout(value);
            }}
            onIdleTimeoutBlur={() => validateResilienceIdleTimeout(resilienceIdleTimeout)}
            idleTimeoutError={resilienceIdleTimeoutError}
          />
        </Grid>

        <Grid size={{ xs: 12 }}>
          <Stack spacing={1.5} width="100%">
            <Collapse in={!!status} timeout={300}>
              {status && (
                <Alert
                  severity={status.severity}
                  onClose={() => setStatus(null)}
                  sx={{ width: "100%" }}
                >
                  {status.message}
                </Alert>
              )}
            </Collapse>
            <Stack direction="row" spacing={1.5} justifyContent="flex-end">
              <Button
                variant="outlined"
                onClick={handleDiscard}
                disabled={!isDirty || isUpdating}
              >
                Discard
              </Button>
              <Button
                variant="contained"
                onClick={() => void handleSave()}
                disabled={
                  isUpdating ||
                  !isDirty ||
                  !!endpointError ||
                  !!resilienceTimeoutError ||
                  !!resilienceIdleTimeoutError
                }
              >
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Stack>
          </Stack>
        </Grid>
      </Grid>
    </Stack>
  );
}

export default MCPProxyConnectionTab;
