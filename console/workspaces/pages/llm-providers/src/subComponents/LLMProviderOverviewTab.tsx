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

import React, {
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import "swagger-ui-react/swagger-ui.css";

const SwaggerUI = React.lazy(() => import("swagger-ui-react"));
import {
  useCreateLLMProviderAPIKey,
  useListGateways,
  useListLLMDeployments,
  useRotateLLMProviderAPIKey,
  useUpdateLLMProvider,
} from "@agent-management-platform/api-client";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  useTheme,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Copy,
  DoorClosedLocked,
  Download,
  Key,
  Save,
} from "@wso2/oxygen-ui-icons-react";
import type { LLMProviderResponse } from "@agent-management-platform/types";
import { parseOpenApiSpec } from "../utils/openapiResources";

const swaggerHideInfoAndServersPlugin = {
  statePlugins: {
    spec: {
      wrapSelectors: {
        servers: () => (): unknown[] => [],
        schemes: () => (): unknown[] => [],
      },
    },
  },
  wrapComponents: {
    info: () => (): null => null,
  },
};

export type LLMProviderOverviewTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl: string | undefined;
  orgName: string | undefined;
  providerId: string | undefined;
  isLoading?: boolean;
  error?: Error | null;
};

function buildInvokeUrl(vhost: string, context: string): string {
  const base = vhost.startsWith("http") ? vhost : `https://${vhost}`;
  const path = context.startsWith("/") ? context : `/${context}`;
  return `${base.replace(/\/$/, "")}${path}`;
}

export function LLMProviderOverviewTab({
  providerData,
  openapiSpecUrl,
  orgName,
  providerId,
  isLoading = false,
  error: providerError = null,
}: LLMProviderOverviewTabProps) {
  const theme = useTheme();
  const [isDownloading, setIsDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const initialOpenapi = providerData?.openapi?.trim() ?? openapiSpecUrl ?? "";
  const [openapiValue, setOpenapiValue] = useState(initialOpenapi);
  const [saveError, setSaveError] = useState<string | null>(null);
  const hasInitializedRef = useRef(false);

  useEffect(() => {
    const saved = (
      providerData?.openapi?.trim() ??
      openapiSpecUrl ??
      ""
    ).trim();
    const current = openapiValue.trim();
    const isUnchanged = current === saved;
    if (isUnchanged) {
      setOpenapiValue(providerData?.openapi?.trim() ?? openapiSpecUrl ?? "");
    } else if (!hasInitializedRef.current && saved) {
      setOpenapiValue(providerData?.openapi?.trim() ?? openapiSpecUrl ?? "");
      hasInitializedRef.current = true;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- openapiValue excluded
  }, [providerData?.openapi, openapiSpecUrl]);

  const updateProvider = useUpdateLLMProvider();

  const handleSaveOpenapi = useCallback(async () => {
    if (!orgName || !providerId) return;
    setSaveError(null);
    try {
      await updateProvider.mutateAsync({
        params: { orgName, providerId },
        body: { openapi: openapiValue.trim() || undefined },
      });
    } catch (err) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save OpenAPI spec",
      );
    }
  }, [orgName, providerId, openapiValue, updateProvider]);

  const hasOpenapiChanged =
    openapiValue.trim() !==
    (providerData?.openapi?.trim() ?? openapiSpecUrl ?? "").trim();

  const swaggerSource = useMemo(() => {
    const v = openapiValue.trim();
    if (!v) return null;
    if (v.startsWith("http://") || v.startsWith("https://")) {
      return { type: "url" as const, value: v };
    }
    const spec = parseOpenApiSpec(v);
    return spec ? { type: "spec" as const, value: spec } : null;
  }, [openapiValue]);

  const { data: deploymentsData } = useListLLMDeployments(
    { orgName: orgName ?? "", providerId: providerId ?? "" },
    { status: "DEPLOYED" },
  );
  const { data: gatewaysData } = useListGateways(
    { orgName: orgName ?? "" },
    { limit: 500 },
  );

  const gatewayOptions = useMemo(() => {
    if (!providerData?.context || !orgName || !providerId) return [];
    const deployments = Array.isArray(deploymentsData) ? deploymentsData : [];
    const gateways = gatewaysData?.gateways ?? [];
    const deployedGatewayIds = new Set(
      deployments.map((d) => d.gatewayId).filter(Boolean),
    );
    return gateways
      .filter((g) => deployedGatewayIds.has(g.uuid))
      .map((g) => ({
        uuid: g.uuid,
        url: buildInvokeUrl(g.vhost, providerData.context),
        displayName: g.displayName || g.name,
      }));
  }, [
    providerData?.context,
    deploymentsData,
    gatewaysData,
    orgName,
    providerId,
  ]);

  const [selectedGatewayId, setSelectedGatewayId] = useState<string>("");
  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);
  const [invokeUrlCopied, setInvokeUrlCopied] = useState(false);

  const selectedGateway = useMemo(
    () => gatewayOptions.find((g) => g.uuid === selectedGatewayId),
    [gatewayOptions, selectedGatewayId],
  );

  const handleCopyInvokeUrl = useCallback(async () => {
    if (!selectedGateway?.url) return;
    try {
      await navigator.clipboard.writeText(selectedGateway.url);
      setInvokeUrlCopied(true);
      setTimeout(() => setInvokeUrlCopied(false), 2000);
    } catch {
      // Silently fail
    }
  }, [selectedGateway?.url]);

  useEffect(() => {
    if (
      gatewayOptions.length > 0 &&
      (!selectedGatewayId ||
        !gatewayOptions.some((g) => g.uuid === selectedGatewayId))
    ) {
      setSelectedGatewayId(gatewayOptions[0].uuid);
    }
  }, [gatewayOptions, selectedGatewayId]);

  const createApiKey = useCreateLLMProviderAPIKey();
  const rotateApiKey = useRotateLLMProviderAPIKey();

  const isApiKeyConflictError = useCallback((err: unknown): boolean => {
    if (err && typeof err === "object") {
      const status =
        (err as { status?: number }).status ??
        (err as { statusCode?: number }).statusCode;
      if (status === 409) return true;
      const msg = String(
        (err as { message?: string }).message ??
          (err as { error?: string }).error ??
          "",
      ).toLowerCase();
      if (
        msg.includes("already exists") ||
        msg.includes("key exists") ||
        msg.includes("conflict")
      ) {
        return true;
      }
    }
    return false;
  }, []);

  const handleGenerateApiKey = useCallback(async () => {
    if (!orgName || !providerId || !selectedGateway) return;
    setApiKeyError(null);
    setGeneratedApiKey(null);
    const keyName = `provider-${providerData?.name}-gateway-${selectedGateway.displayName}`;
    try {
      const res = await createApiKey.mutateAsync({
        params: { orgName, providerId },
        body: {
          name: keyName,
          displayName: keyName,
        },
      });
      if (res.apiKey) setGeneratedApiKey(res.apiKey);
    } catch (createErr) {
      if (isApiKeyConflictError(createErr)) {
        try {
          const res = await rotateApiKey.mutateAsync({
            params: { orgName, providerId, keyName },
            body: {},
          });
          if (res.apiKey) setGeneratedApiKey(res.apiKey);
        } catch (rotateErr) {
          setApiKeyError(
            rotateErr instanceof Error
              ? rotateErr.message
              : "Failed to rotate API key",
          );
        }
      } else {
        setApiKeyError(
          createErr instanceof Error
            ? createErr.message
            : "Failed to generate API key",
        );
      }
    }
  }, [
    orgName,
    providerId,
    selectedGateway,
    createApiKey,
    rotateApiKey,
    isApiKeyConflictError,
  ]);

  const handleDownload = useCallback(async () => {
    const urlToFetch = openapiValue.trim().startsWith("http")
      ? openapiValue.trim()
      : null;
    if (!urlToFetch) return;
    setIsDownloading(true);
    setDownloadError(null);
    try {
      const res = await fetch(urlToFetch);
      if (!res.ok) {
        throw new Error(
          `Failed to download spec: ${res.status} ${res.statusText}`,
        );
      }
      const text = await res.text();
      const ext = urlToFetch.endsWith(".json") ? "json" : "yaml";
      const blob = new Blob([text], {
        type: ext === "json" ? "application/json" : "text/yaml",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `openapi-spec.${ext}`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setDownloadError(
        err instanceof Error ? err.message : "Failed to download spec.",
      );
    } finally {
      setIsDownloading(false);
    }
  }, [openapiValue]);

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Grid container spacing={2}>
          {[1, 2, 3, 4, 5].map((i) => (
            <Grid key={i} size={{ xs: 12, sm: 6, md: 4 }}>
              <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                <Stack spacing={1}>
                  <Skeleton variant="text" width="40%" height={16} />
                  <Skeleton variant="text" width="80%" height={20} />
                </Stack>
              </Card>
            </Grid>
          ))}
        </Grid>
        <Divider />
        <Stack spacing={1.5} sx={{ mt: 3 }}>
          <Skeleton variant="text" width={140} height={20} />
          <Stack direction="row" spacing={1} alignItems="center">
            <Skeleton variant="rounded" height={40} sx={{ flex: 1 }} />
            <Skeleton variant="rounded" width={120} height={40} />
          </Stack>
          <Skeleton variant="rounded" height={400} />
        </Stack>
      </Stack>
    );
  }

  if (!providerData && !providerError) {
    return null;
  }

  if (providerError && !isLoading) {
    return (
      <Alert severity="error" sx={{ width: "100%" }}>
        {providerError instanceof Error
          ? providerError.message
          : "Failed to load provider."}
      </Alert>
    );
  }

  if (!providerData) {
    return null;
  }

  return (
    <Stack spacing={3}>
      <Grid container spacing={2}>
        {providerData.context && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Context
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    fontFamily:
                      (theme.typography as { fontFamilyMonospace?: string })
                        ?.fontFamilyMonospace ?? "monospace",
                  }}
                >
                  {providerData.context}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        {providerData.upstream?.main?.url && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Upstream URL
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    fontFamily:
                      (theme.typography as { fontFamilyMonospace?: string })
                        ?.fontFamilyMonospace ?? "monospace",
                    wordBreak: "break-all",
                  }}
                >
                  {providerData.upstream.main.url}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        {providerData.upstream?.main?.auth?.type && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Auth Type
                </Typography>
                <Typography variant="body2">
                  {providerData.upstream.main.auth.type}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        {providerData.accessControl?.mode && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Access Control
                </Typography>
                <Chip
                  label={providerData.accessControl.mode}
                  size="small"
                  variant="outlined"
                  sx={{
                    width: "fit-content",
                    textTransform: "capitalize",
                  }}
                />
              </Stack>
            </Card>
          </Grid>
        )}
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
            <Stack spacing={0.5}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontWeight: 500 }}
              >
                In Catalog
              </Typography>
              <Chip
                label={providerData.inCatalog ? "Yes" : "No"}
                size="small"
                color={providerData.inCatalog ? "success" : "default"}
                variant="outlined"
                sx={{ width: "fit-content" }}
              />
            </Stack>
          </Card>
        </Grid>
      </Grid>
      {/* Invoke URLs & API Key section */}
      {orgName && providerId && (
        <Stack spacing={2} sx={{ mt: 2 }}>
          <Stack
            direction="row"
            alignItems="center"
            flexWrap="wrap"
            gap={1}
          >
            <Typography
              variant="subtitle2"
              color="text.secondary"
              sx={{ fontWeight: 600 }}
            >
              Invoke URL & API Key
            </Typography>
            {gatewayOptions.length > 0 && (
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <Select
                  value={selectedGatewayId || ""}
                  onChange={(e) => {
                    const id = String(e.target.value ?? "");
                    setSelectedGatewayId(id);
                    setGeneratedApiKey(null);
                    setApiKeyError(null);
                  }}
                  size="small"
                  displayEmpty
                >
                  {gatewayOptions.map((g) => (
                    <MenuItem key={g.uuid} value={g.uuid}>
                      <Stack direction="row" alignItems="center" gap={1}>
                      <DoorClosedLocked size={16} />
                      {g.displayName || g.uuid}
                      </Stack>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
          </Stack>
          {gatewayOptions.length > 0 ? (
            <Card variant="outlined" sx={{ p: 2 }}>
              <Stack spacing={2}>
                {selectedGateway && (
                  <>
                    <FormControl fullWidth size="small">
                      <FormLabel>Invoke URL</FormLabel>
                      <TextField
                        size="small"
                        fullWidth
                        key={selectedGatewayId}
                        value={selectedGateway.url}
                        slotProps={{
                          input: {
                            readOnly: true,
                            endAdornment: (
                              <InputAdornment position="end">
                                <Tooltip
                                  title={invokeUrlCopied ? "Copied!" : "Copy"}
                                >
                                  <IconButton
                                    size="small"
                                    onClick={handleCopyInvokeUrl}
                                    aria-label="Copy Invoke URL"
                                  >
                                    <Copy size={16} />
                                  </IconButton>
                                </Tooltip>
                              </InputAdornment>
                            ),
                          },
                        }}
                        sx={{
                          "& .MuiInputBase-input": {
                            fontFamily:
                              (theme.typography as { fontFamilyMonospace?: string })
                                ?.fontFamilyMonospace ?? "monospace",
                            fontSize:
                              theme.typography.body2?.fontSize,
                            wordBreak: "break-all",
                          },
                        }}
                      />
                    </FormControl>
                    <Stack spacing={1}>
                      <FormLabel>Generate API Key</FormLabel>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Button
                          variant="outlined"
                          size="medium"
                          startIcon={<Key size={16} />}
                          onClick={handleGenerateApiKey}
                          disabled={
                            createApiKey.isPending || rotateApiKey.isPending
                          }
                        >
                          {createApiKey.isPending || rotateApiKey.isPending
                            ? "Generating..."
                            : "Generate API Key"}
                        </Button>
                      </Stack>
                      {apiKeyError && (
                        <Alert
                          severity="error"
                          onClose={() => setApiKeyError(null)}
                        >
                          {apiKeyError}
                        </Alert>
                      )}
                      {generatedApiKey && (
                        <TextField
                          size="small"
                          fullWidth
                          label="API Key (copy now — shown only once)"
                          value={generatedApiKey}
                          slotProps={{ input: { readOnly: true } }}
                          sx={{
                            "& .MuiInputBase-input": {
                              fontFamily:
                                (theme.typography as { fontFamilyMonospace?: string })
                                  ?.fontFamilyMonospace ?? "monospace",
                              fontSize:
                                theme.typography.body2?.fontSize,
                              wordBreak: "break-all",
                            },
                          }}
                        />
                      )}
                    </Stack>
                  </>
                )}
              </Stack>
            </Card>
          ) : (
            <Alert severity="info">
              No invoke URLs available. Deploy this provider to an AI gateway to
              see invoke URLs and generate API keys.
            </Alert>
          )}
        </Stack>
      )}
      <Divider />
      {/* OpenAPI Resources section */}
      <Stack spacing={1.5} sx={{ mt: 3 }}>
        <Typography
          variant="subtitle2"
          color="text.secondary"
          sx={{ fontWeight: 600 }}
        >
          OpenAPI Resources
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Enter a URL and Click Save to
          persist changes to the provider.
        </Typography>
        <Stack direction="row" spacing={1} alignItems="flex-start">
          <TextField
            size="small"
            fullWidth
            value={openapiValue}
            onChange={(e) => {
              setOpenapiValue(e.target.value);
              setSaveError(null);
            }}
            placeholder="https://example.com/openapi.json"
            sx={{
              "& .MuiInputBase-input": {
                fontFamily:
                  (theme.typography as { fontFamilyMonospace?: string })
                    ?.fontFamilyMonospace ?? "monospace",
                fontSize: theme.typography.body2?.fontSize,
              },
            }}
          />
          <Stack direction="row" spacing={1} flexShrink={0}>
            <Button
              variant="contained"
              size="medium"
              startIcon={<Save size={16} />}
              onClick={handleSaveOpenapi}
              disabled={
                !hasOpenapiChanged ||
                updateProvider.isPending ||
                !orgName ||
                !providerId
              }
            >
              {updateProvider.isPending ? "Saving..." : "Save"}
            </Button>
            <Button
              variant="outlined"
              size="medium"
              startIcon={<Download size={16} />}
              onClick={handleDownload}
              disabled={
                isDownloading || !openapiValue.trim().startsWith("http")
              }
            >
              {isDownloading ? "Downloading..." : "Download"}
            </Button>
          </Stack>
        </Stack>
        {saveError && (
          <Alert severity="error" onClose={() => setSaveError(null)}>
            {saveError}
          </Alert>
        )}
        {downloadError && (
          <Alert severity="error" onClose={() => setDownloadError(null)}>
            {downloadError}
          </Alert>
        )}
        {swaggerSource ? (
          <Stack spacing={1}>
            <Accordion disableGutters>
              <AccordionSummary expandIcon={<ChevronDown size={18} />}>
                <Typography variant="subtitle2" color="text.secondary">
                  API Preview
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Suspense
                  fallback={
                    <Stack spacing={1} sx={{ py: 3 }}>
                      <Skeleton variant="rounded" height={48} />
                      <Skeleton variant="rounded" height={200} />
                      <Skeleton variant="rounded" height={400} />
                    </Stack>
                  }
                >
                  <Box
                    className="hide-scheme-container hide-models swagger-spec-viewer hide-info-section hide-servers hide-authorize hide-operation-header"
                    sx={{
                      "& .swagger-ui .wrapper": { padding: 0 },
                      "&.hide-info-section .swagger-ui .info": {
                        display: "none !important",
                      },
                      "&.hide-servers .swagger-ui .servers, &.hide-servers .swagger-ui .schemes":
                        { display: "none !important" },
                      "&.hide-authorize .swagger-ui .auth-wrapper": {
                        display: "none !important",
                      },
                      "&.hide-tag-headers .swagger-ui .opblock-tag-section": {
                        display: "none !important",
                      },
                      "&.hide-operation-header .swagger-ui .opblock-section-header":
                        { display: "none !important" },
                      "&.hide-scheme-container .swagger-ui .scheme-container": {
                        display: "none !important",
                      },
                      "&.hide-models .swagger-ui .models": {
                        display: "none !important",
                      },
                    }}
                  >
                    <SwaggerUI
                      {...(swaggerSource.type === "url"
                        ? { url: swaggerSource.value }
                        : { spec: swaggerSource.value })}
                      layout="BaseLayout"
                      docExpansion="list"
                      plugins={[swaggerHideInfoAndServersPlugin]}
                    />
                  </Box>
                </Suspense>
              </AccordionDetails>
            </Accordion>
          </Stack>
        ) : (
          <Alert severity="info">
            Enter a URL or paste an OpenAPI spec above to preview it here.
          </Alert>
        )}
      </Stack>
    </Stack>
  );
}
