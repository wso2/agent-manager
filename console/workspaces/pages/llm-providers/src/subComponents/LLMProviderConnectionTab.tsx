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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { z } from "zod";
import {
  type LLMProviderResponse,
  type UpdateLLMProviderRequest,
  type UpstreamAuthType,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  Alert,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
} from "@wso2/oxygen-ui";
import { Eye, EyeOff } from "@wso2/oxygen-ui-icons-react";
import { ResilienceTimeoutFields } from "@agent-management-platform/shared-component";

const MASKED_CREDENTIAL_VALUE = "••••••••••••";

const providerEndpointSchema = z
  .string()
  .min(1, "Provider Endpoint is required")
  .url("Please enter a valid URL");

const DURATION_PATTERN = /^\d+(ms|s|m|h)$/;

const resilienceTimeoutSchema = z
  .string()
  .refine((value) => value.trim() === "" || DURATION_PATTERN.test(value.trim()), {
    message: "Enter a duration like 5s, 500ms, or 1m",
  });

const AUTH_TYPE_API_KEY = "api-key";

// "bearer" and "basic" are absent from the gateway's upstream auth enum, so they never deploy.
const AUTH_TYPE_OPTIONS: { value: UpstreamAuthType; label: string }[] = [
  { value: "api-key", label: "API Key" },
  { value: "none", label: "None" },
];

export type LLMProviderConnectionTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  valuePrefix?: string;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderConnectionTab({
  providerData,
  valuePrefix = "",
  isLoading = false,
  onUpdate,
  isUpdating,
}: LLMProviderConnectionTabProps) {
  const initializedProviderIdRef = useRef<string | null>(null);
  const userActuallyTypedCredentialRef = useRef(false);

  const [providerEndpoint, setProviderEndpoint] = useState("");
  const [authenticationType, setAuthenticationType] =
    useState<UpstreamAuthType>(AUTH_TYPE_API_KEY);
  const [authenticationHeader, setAuthenticationHeader] = useState("");
  const [credentialValue, setCredentialValue] = useState("");
  const [isCredentialMasked, setIsCredentialMasked] = useState(false);
  const [showCredential, setShowCredential] = useState(false);
  const [resilienceTimeout, setResilienceTimeout] = useState("");
  const [resilienceIdleTimeout, setResilienceIdleTimeout] = useState("");
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [endpointError, setEndpointError] = useState<string | null>(null);
  const [resilienceTimeoutError, setResilienceTimeoutError] = useState<string | null>(null);
  const [resilienceIdleTimeoutError, setResilienceIdleTimeoutError] = useState<string | null>(
    null,
  );

  useEffect(() => {
    if (!providerData) return;
    const providerUuid = providerData.uuid;
    if (initializedProviderIdRef.current === providerUuid) return;
    initializedProviderIdRef.current = providerUuid;
    setProviderEndpoint(providerData.upstream?.main?.url ?? "");
    setAuthenticationType(
      (providerData.upstream?.main?.auth?.type as UpstreamAuthType) ?? AUTH_TYPE_API_KEY,
    );
    setAuthenticationHeader(providerData.upstream?.main?.auth?.header ?? "");
    setCredentialValue(MASKED_CREDENTIAL_VALUE);
    setIsCredentialMasked(true);
    setResilienceTimeout(providerData.resilience?.timeout ?? "");
    setResilienceIdleTimeout(providerData.resilience?.idleTimeout ?? "");
    setEndpointError(null);
    setResilienceTimeoutError(null);
    setResilienceIdleTimeoutError(null);
  }, [providerData]);

  const isDirty = useMemo(() => {
    if (!providerData) return false;
    const main = providerData.upstream?.main;
    const savedUrl = (main?.url ?? "").trim();
    const savedAuthType = (main?.auth?.type as UpstreamAuthType) ?? AUTH_TYPE_API_KEY;
    const savedAuthHeader = (main?.auth?.header ?? "").trim();
    const savedResilienceTimeout = (providerData.resilience?.timeout ?? "").trim();
    const savedResilienceIdleTimeout = (providerData.resilience?.idleTimeout ?? "").trim();

    if (providerEndpoint.trim() !== savedUrl) return true;
    if ((authenticationType || AUTH_TYPE_API_KEY) !== savedAuthType) return true;
    if (authenticationHeader.trim() !== savedAuthHeader) return true;
    if (resilienceTimeout.trim() !== savedResilienceTimeout) return true;
    if (resilienceIdleTimeout.trim() !== savedResilienceIdleTimeout) return true;

    if (
      !isCredentialMasked &&
      credentialValue.trim() !== MASKED_CREDENTIAL_VALUE
    ) {
      return true;
    }
    return false;
  }, [
    providerData,
    providerEndpoint,
    authenticationType,
    authenticationHeader,
    resilienceTimeout,
    resilienceIdleTimeout,
    credentialValue,
    isCredentialMasked,
  ]);

  const validateEndpoint = useCallback((value: string): string | null => {
    const result = providerEndpointSchema.safeParse(value.trim());
    if (result.success) {
      setEndpointError(null);
      return null;
    }
    const err = result.error.flatten().formErrors[0] ?? "Invalid URL";
    setEndpointError(err);
    return err;
  }, []);

  const validateResilienceTimeout = useCallback((value: string): string | null => {
    const result = resilienceTimeoutSchema.safeParse(value);
    if (result.success) {
      setResilienceTimeoutError(null);
      return null;
    }
    const err = result.error.flatten().formErrors[0] ?? "Invalid duration";
    setResilienceTimeoutError(err);
    return err;
  }, []);

  const validateResilienceIdleTimeout = useCallback((value: string): string | null => {
    const result = resilienceTimeoutSchema.safeParse(value);
    if (result.success) {
      setResilienceIdleTimeoutError(null);
      return null;
    }
    const err = result.error.flatten().formErrors[0] ?? "Invalid duration";
    setResilienceIdleTimeoutError(err);
    return err;
  }, []);

  const handleDiscard = useCallback(() => {
    if (!providerData) return;
    setProviderEndpoint(providerData.upstream?.main?.url ?? "");
    setAuthenticationType(
      (providerData.upstream?.main?.auth?.type as UpstreamAuthType) ?? AUTH_TYPE_API_KEY,
    );
    setAuthenticationHeader(providerData.upstream?.main?.auth?.header ?? "");
    setCredentialValue(MASKED_CREDENTIAL_VALUE);
    setIsCredentialMasked(true);
    setResilienceTimeout(providerData.resilience?.timeout ?? "");
    setResilienceIdleTimeout(providerData.resilience?.idleTimeout ?? "");
    setEndpointError(null);
    setResilienceTimeoutError(null);
    setResilienceIdleTimeoutError(null);
    setStatus(null);
  }, [providerData]);

  const handleSave = useCallback(async () => {
    if (!providerData) return;

    const nextUrl = providerEndpoint.trim();
    const endpointValidationError = validateEndpoint(providerEndpoint);
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

    let authValue = providerData.upstream?.main?.auth?.value ?? "";
    if (
      !isCredentialMasked &&
      credentialValue.trim() !== MASKED_CREDENTIAL_VALUE
    ) {
      if (
        credentialValue.trim() === "" &&
        !userActuallyTypedCredentialRef.current
      ) {
        authValue = providerData.upstream?.main?.auth?.value ?? "";
      } else {
        const nextValue = credentialValue.trim();
        authValue = valuePrefix
          ? nextValue.startsWith(valuePrefix)
            ? nextValue
            : `${valuePrefix}${nextValue}`
          : nextValue;
      }
    }

    const authPayload =
      authenticationType === "none"
        ? { type: "none" as const, header: "", value: "" }
        : {
            type: (authenticationType || AUTH_TYPE_API_KEY) as UpstreamAuthType,
            header: authenticationHeader.trim() || "",
            value: authValue,
          };

    const trimmedResilienceTimeout = resilienceTimeout.trim();
    const trimmedResilienceIdleTimeout = resilienceIdleTimeout.trim();

    try {
      await onUpdate({
        upstream: {
          main: {
            url: nextUrl,
            auth: authPayload,
          },
        },
        resilience:
          trimmedResilienceTimeout || trimmedResilienceIdleTimeout
            ? {
                timeout: trimmedResilienceTimeout || undefined,
                idleTimeout: trimmedResilienceIdleTimeout || undefined,
              }
            : undefined,
        // Passing the provider's currently-deployed gateways (even unchanged) makes the
        // backend take the UpdateAndSync path, which regenerates and re-pushes the
        // deployment YAML to those gateways. Without this, a Connection tab save only
        // updates the stored config — already-deployed gateways never see the new values.
        gateways:
          providerData.gateways && providerData.gateways.length > 0
            ? providerData.gateways
            : undefined,
      });
      setStatus({
        message: "Connection updated successfully.",
        severity: "success",
      });
      if (
        !isCredentialMasked &&
        credentialValue.trim() !== MASKED_CREDENTIAL_VALUE
      ) {
        setCredentialValue(MASKED_CREDENTIAL_VALUE);
        setIsCredentialMasked(true);
      }
    } catch {
      setStatus({ message: "Failed to update connection.", severity: "error" });
    }
  }, [
    providerData,
    providerEndpoint,
    authenticationType,
    authenticationHeader,
    resilienceTimeout,
    resilienceIdleTimeout,
    credentialValue,
    valuePrefix,
    isCredentialMasked,
    onUpdate,
    validateEndpoint,
    validateResilienceTimeout,
    validateResilienceIdleTimeout,
  ]);

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Stack spacing={2}>
          {[1, 2, 3, 4].map((i) => (
            <Stack key={i} spacing={0.5}>
              <Skeleton variant="text" width={120} height={16} />
              <Skeleton variant="rounded" height={40} />
            </Stack>
          ))}
        </Stack>
      </Stack>
    );
  }

  if (!providerData) {
    return null;
  }

  return (
    <>
      <Stack spacing={2}>
        <Grid container spacing={3}>
          <Grid size={{ xs: 12 }}>
            <FormControl fullWidth>
              <FormLabel>Provider Endpoint</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
                size="small"
                value={providerEndpoint}
                onChange={(e) => {
                  setProviderEndpoint(e.target.value);
                  if (endpointError) validateEndpoint(e.target.value);
                }}
                onBlur={() => validateEndpoint(providerEndpoint)}
                error={!!endpointError}
                helperText={endpointError}
                sx={{
                  "& .MuiInputBase-input": {
                    fontFamily: "monospace",
                    fontSize: "0.875rem",
                  },
                }}
              />
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12, sm: 6 }}>
            <FormControl fullWidth>
              <FormLabel>Authentication</FormLabel>
              <Select
                size="small"
                value={authenticationType || AUTH_TYPE_API_KEY}
                onChange={(e) =>
                  setAuthenticationType(e.target.value as UpstreamAuthType)
                }
              >
                {AUTH_TYPE_OPTIONS.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12, sm: 6 }}>
            <FormControl fullWidth>
              <FormLabel>Authentication Header</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.KEY } }}
                size="small"
                value={authenticationHeader}
                onChange={(e) => setAuthenticationHeader(e.target.value)}
              />
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <FormControl fullWidth>
              <FormLabel>Credentials</FormLabel>
              <TextField
                size="small"
                type={showCredential ? "text" : "password"}
                value={credentialValue}
                onFocus={() => {
                  if (isCredentialMasked) {
                    setCredentialValue("");
                    setIsCredentialMasked(false);
                    userActuallyTypedCredentialRef.current = false;
                  }
                }}
                onChange={(e) => {
                  userActuallyTypedCredentialRef.current = true;
                  setCredentialValue(e.target.value);
                }}
                slotProps={{
                  htmlInput: { maxLength: INPUT_LIMITS.SECRET },
                  input: {
                    endAdornment: (
                      <InputAdornment position="end">
                        <IconButton
                          size="small"
                          onClick={() => setShowCredential((p) => !p)}
                          aria-label={
                            showCredential
                              ? "Hide credentials"
                              : "Show credentials"
                          }
                        >
                          {showCredential ? (
                            <EyeOff size={18} />
                          ) : (
                            <Eye size={18} />
                          )}
                        </IconButton>
                      </InputAdornment>
                    ),
                  },
                }}
                sx={{
                  "& .MuiInputBase-input": {
                    fontFamily: "monospace",
                  },
                }}
              />
            </FormControl>
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
            <Stack spacing={1.5} width="100%" >
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
    </>
  );
}
