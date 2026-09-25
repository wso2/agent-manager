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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  type LLMProviderResponse,
  type UpdateLLMProviderRequest,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import { z } from "zod";
import {
  Alert,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";

// The api-key-auth policy declares `in` as enum: ["header"], so a location choice here
// produced providers the gateway could never authenticate. Fixed, not selectable.
const API_KEY_LOCATION = "header";

type StoredAPIKeyConfig = { enabled?: boolean; key?: string; in?: string };

/**
 * The saved security config, mirroring SecurityConfig.RequiresAPIKey on the backend so
 * the form never shows "apiKey" for a provider provisioned without one.
 */
function readStoredSecurity(providerData: LLMProviderResponse) {
  const apiKeyConfig = providerData.security?.apiKey as StoredAPIKeyConfig | undefined;
  const key = (apiKeyConfig?.key ?? "").trim();
  const securityEnabled = providerData.security?.enabled !== false;
  const hasApiKey = securityEnabled && apiKeyConfig?.enabled === true && !!key;
  return {
    authenticationType: hasApiKey ? ("apiKey" as const) : ("none" as const),
    key: apiKeyConfig?.key ?? "",
    in: apiKeyConfig?.in ?? API_KEY_LOCATION,
  };
}

const securityFormSchema = z
  .object({
    authenticationType: z.enum(["apiKey", "none"]),
    keyValue: z.string(),
  })
  .refine(
    (data) => {
      if (data.authenticationType === "apiKey") {
        return data.keyValue.trim().length > 0;
      }
      return true;
    },
    { message: "API Key is required when using API Key authentication", path: ["keyValue"] }
  );

export type LLMProviderSecurityTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderSecurityTab({
  providerData,
  isLoading = false,
  onUpdate,
  isUpdating,
}: LLMProviderSecurityTabProps) {

  const [authenticationType, setAuthenticationType] = useState<
    "apiKey" | "none"
  >("apiKey");
  const [keyValue, setKeyValue] = useState("");
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ keyValue?: string }>({});

  const isDirty = useMemo(() => {
    if (!providerData) return false;
    const saved = readStoredSecurity(providerData);
    if (authenticationType !== saved.authenticationType) return true;
    if (keyValue.trim() !== saved.key) return true;
    // A provider still stored with a non-header location can no longer be expressed
    // by this form, so surface it as unsaved — saving rewrites it to a header and
    // repairs a proxy the gateway was rejecting every request for.
    return saved.authenticationType === "apiKey" && saved.in !== API_KEY_LOCATION;
  }, [providerData, authenticationType, keyValue]);

  const resetToSaved = useCallback(() => {
    if (!providerData) return;
    const saved = readStoredSecurity(providerData);
    setAuthenticationType(saved.authenticationType);
    setKeyValue(saved.key);
    setFieldErrors({});
  }, [providerData]);

  useEffect(() => {
    resetToSaved();
  }, [resetToSaved]);

  const handleDiscard = useCallback(() => {
    resetToSaved();
    setStatus(null);
  }, [resetToSaved]);

  const handleSave = useCallback(async () => {
    if (!providerData) return;

    const result = securityFormSchema.safeParse({
      authenticationType,
      keyValue,
    });

    if (!result.success) {
      const flatten = result.error.flatten();
      const firstError = flatten.formErrors[0];
      setFieldErrors({
        keyValue: flatten.fieldErrors.keyValue?.[0],
      });
      setStatus({
        message: firstError ?? "Validation failed.",
        severity: "error",
      });
      return;
    }
    setFieldErrors({});

    const nextKey = result.data.keyValue.trim();

    try {
      await onUpdate({
        security: {
          enabled: providerData.security?.enabled ?? true,
          apiKey: {
            enabled: authenticationType === "apiKey",
            key: authenticationType === "apiKey" ? nextKey : "",
            in: API_KEY_LOCATION,
          },
        },
      });
      setFieldErrors({});
      setStatus({
        message: "Updated security settings.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update security.",
        severity: "error",
      });
    }
  }, [
    providerData,
    authenticationType,
    keyValue,
    onUpdate,
  ]);

  const isDisabled = isLoading || !providerData;

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Typography variant="h6">
          Authentication
        </Typography>
        <Stack spacing={2}>
          {[1, 2, 3].map((i) => (
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
    <Stack spacing={2}>
      <Typography variant="h6">
        Authentication
      </Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 5 }}>
          <FormControl fullWidth disabled={isDisabled}>
            <FormLabel>Method</FormLabel>
            <Select
              size="small"
              value={authenticationType}
              onChange={(e) =>
                setAuthenticationType(e.target.value as "apiKey" | "none")
              }
            >
              {/* "none" rather than "": MUI renders an empty-string value as no
                  selection at all, which left the field blank after picking None. */}
              <MenuItem value="none">None</MenuItem>
              <MenuItem value="apiKey">apiKey</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      {authenticationType === "apiKey" && (
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 5 }}>
            <FormControl fullWidth disabled={isDisabled} error={!!fieldErrors.keyValue}>
              <FormLabel>Header Key</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.KEY } }}
                size="small"
                value={keyValue}
                onChange={(e) => {
                  setKeyValue(e.target.value);
                  if (fieldErrors.keyValue) setFieldErrors({});
                }}
                error={!!fieldErrors.keyValue}
                helperText={fieldErrors.keyValue}
                sx={{
                  "& .MuiInputBase-input": {
                    fontFamily: "monospace",
                  },
                }}
              />
            </FormControl>
          </Grid>
        </Grid>
      )}

      <Stack spacing={1.5} width="100%">
        <Collapse in={!!status && !isDirty} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%", maxWidth: 480 }}
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
            disabled={isUpdating || !isDirty}
          >
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
