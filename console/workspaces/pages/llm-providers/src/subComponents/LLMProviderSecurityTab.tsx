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
import type {
  APIKeyLocation,
  LLMProviderResponse,
  UpdateLLMProviderRequest,
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

const KEY_LOCATION_OPTIONS: { value: APIKeyLocation; label: string }[] = [
  { value: "header", label: "header" },
  { value: "query", label: "query" },
];

const securityFormSchema = z
  .object({
    authenticationType: z.enum(["apiKey", ""]),
    keyValue: z.string(),
    keyIn: z.enum(["header", "query"]),
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
    "apiKey" | ""
  >("apiKey");
  const [keyValue, setKeyValue] = useState("");
  const [keyIn, setKeyIn] = useState<APIKeyLocation>("header");
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ keyValue?: string }>({});

  const isDirty = useMemo(() => {
    if (!providerData) return false;
    const apiKeyConfig = providerData.security?.apiKey as
      | { enabled?: boolean; key?: string; in?: string }
      | undefined;
    const hasApiKey =
      !!apiKeyConfig &&
      apiKeyConfig.enabled !== false &&
      !!(apiKeyConfig.key ?? "").trim();
    const savedType = hasApiKey ? "apiKey" : "";
    const savedKey = providerData.security?.apiKey?.key ?? "";
    const savedIn =
      (providerData.security?.apiKey?.in as APIKeyLocation) ?? "header";
    if (authenticationType !== savedType) return true;
    if (keyValue.trim() !== savedKey) return true;
    if (keyIn !== savedIn) return true;
    return false;
  }, [providerData, authenticationType, keyValue, keyIn]);

  useEffect(() => {
    if (!providerData) return;
    const apiKeyConfig = providerData.security?.apiKey as
      | { enabled?: boolean; key?: string; in?: string }
      | undefined;
    const hasApiKey =
      !!apiKeyConfig &&
      apiKeyConfig.enabled !== false &&
      !!(apiKeyConfig.key ?? "").trim();
    setAuthenticationType(hasApiKey ? "apiKey" : "");
    setKeyValue(providerData.security?.apiKey?.key ?? "");
    setKeyIn(
      (providerData.security?.apiKey?.in as APIKeyLocation) ?? "header",
    );
    setFieldErrors({});
  }, [providerData]);

  const handleDiscard = useCallback(() => {
    if (!providerData) return;
    const apiKeyConfig = providerData.security?.apiKey as
      | { enabled?: boolean; key?: string; in?: string }
      | undefined;
    const hasApiKey =
      !!apiKeyConfig &&
      apiKeyConfig.enabled !== false &&
      !!(apiKeyConfig.key ?? "").trim();
    setAuthenticationType(hasApiKey ? "apiKey" : "");
    setKeyValue(providerData.security?.apiKey?.key ?? "");
    setKeyIn(
      (providerData.security?.apiKey?.in as APIKeyLocation) ?? "header",
    );
    setFieldErrors({});
    setStatus(null);
  }, [providerData]);

  const handleSave = useCallback(async () => {
    if (!providerData) return;

    const result = securityFormSchema.safeParse({
      authenticationType: authenticationType || "",
      keyValue,
      keyIn,
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
    const nextIn = result.data.keyIn;

    try {
      await onUpdate({
        security: {
          enabled: providerData.security?.enabled ?? true,
          apiKey: {
            enabled: authenticationType === "apiKey",
            key: authenticationType === "apiKey" ? nextKey : "",
            in: nextIn,
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
    keyIn,
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
            <FormLabel>Authentication</FormLabel>
            <Select
              size="small"
              value={authenticationType || ""}
              onChange={(e) =>
                setAuthenticationType(
                  (e.target.value as "apiKey" | "") || "",
                )
              }
            >
              <MenuItem value="">None</MenuItem>
              <MenuItem value="apiKey">apiKey</MenuItem>
            </Select>
          </FormControl>
        </Grid>

        {authenticationType === "apiKey" && (
          <>
            <Grid size={{ xs: 12, md: 5 }}>
              <FormControl fullWidth disabled={isDisabled} error={!!fieldErrors.keyValue}>
                <FormLabel>Header Key</FormLabel>
                <TextField
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
            <Grid size={{ xs: 12, md: 5 }}>
              <FormControl fullWidth disabled={isDisabled}>
                <FormLabel>Key Location</FormLabel>
                <Select
                  size="small"
                  value={keyIn}
                  onChange={(e) =>
                    setKeyIn(e.target.value as APIKeyLocation)
                  }
                >
                  {KEY_LOCATION_OPTIONS.map((opt) => (
                    <MenuItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
          </>
        )}
      </Grid>

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
