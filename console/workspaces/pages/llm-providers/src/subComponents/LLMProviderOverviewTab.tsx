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

import {
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  InvokeEndpoints,
  type InvokeEndpoint,
  SwaggerSpecViewer,
} from "@agent-management-platform/shared-component";
import {
  useCreateLLMProviderAPIKey,
  useListGateways,
  useListLLMDeployments,
  useRotateLLMProviderAPIKey,
} from "@agent-management-platform/api-client";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Button,
  Card,
  Chip,
  Divider,
  Grid,
  Link,
  useTheme,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Download,
  Save,
} from "@wso2/oxygen-ui-icons-react";
import {
  type LLMProviderResponse,
  type UpdateLLMProviderRequest,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import { parseOpenApiSpec } from "../utils/openapiResources";

export type LLMProviderOverviewTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl: string | undefined;
  orgName: string | undefined;
  providerId: string | undefined;
  isLoading?: boolean;
  error?: Error | null;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
  onGoToDeploymentTab?: () => void;
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
  onUpdate,
  isUpdating,
  onGoToDeploymentTab,
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

  const handleSaveOpenapi = useCallback(async () => {
    setSaveError(null);
    try {
      await onUpdate({ openapi: openapiValue.trim() || undefined });
    } catch (err) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save OpenAPI spec",
      );
    }
  }, [openapiValue, onUpdate]);

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
  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgName ?? "" },
    { limit: 500 },
  );

  const endpoints = useMemo<InvokeEndpoint[]>(() => {
    if (!providerData?.context || !orgName || !providerId) return [];
    const deployments = Array.isArray(deploymentsData) ? deploymentsData : [];
    const gateways = gatewaysData?.gateways ?? [];
    const deployedGatewayIds = new Set(
      deployments.map((d) => d.gatewayId).filter(Boolean),
    );
    return gateways
      .filter((g) => deployedGatewayIds.has(g.uuid))
      .map((g) => ({
        gatewayId: g.uuid,
        url: buildInvokeUrl(g.vhost, providerData.context),
        displayName: g.displayName,
        name: g.name,
        environment: g.environments?.[0],
      }));
  }, [
    providerData?.context,
    deploymentsData,
    gatewaysData,
    orgName,
    providerId,
  ]);

  const deployedPlacement = useMemo(() => {
    const deployedIds = new Set(providerData?.gateways ?? []);
    const allGateways = gatewaysData?.gateways ?? [];
    const knownIds = new Set(allGateways.map((g) => g.uuid));
    const environmentNames = new Set<string>();
    // A deployed UUID absent from the fetched gateway list (deleted gateway,
    // truncated page) still gets a chip rather than vanishing from the card.
    const unmappedGatewayUuids = [...deployedIds].filter(
      (uuid) => !knownIds.has(uuid),
    );
    allGateways.forEach((g) => {
      if (!deployedIds.has(g.uuid)) return;
      const environments = g.environments ?? [];
      if (environments.length === 0) {
        unmappedGatewayUuids.push(g.uuid);
        return;
      }
      environments.forEach((env) =>
        environmentNames.add(env.displayName || env.name),
      );
    });
    return {
      environmentNames: [...environmentNames],
      unmappedGatewayUuids,
    };
  }, [providerData?.gateways, gatewaysData]);

  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);

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
    if (!orgName || !providerId || endpoints.length === 0) return;
    setApiKeyError(null);
    setGeneratedApiKey(null);
    const randomSuffix = Math.random().toString(36).slice(2, 10);
    const keyName = `provider-${providerData?.id ?? "unknown"}-${randomSuffix}`;
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
    endpoints.length,
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
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
            <Stack spacing={0.5}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontWeight: 500 }}
              >
                Deployment
              </Typography>
              {/* Until the gateway list arrives, every deployed UUID would
                  wrongly render as "Unmapped". */}
              {(providerData.gateways ?? []).length > 0 && isLoadingGateways && (
                <Skeleton variant="rounded" width={120} height={24} />
              )}
              {(providerData.gateways ?? []).length > 0 && !isLoadingGateways && (
                <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                  {deployedPlacement.environmentNames.map((name) => (
                    <Chip
                      key={name}
                      label={name}
                      size="small"
                      variant="outlined"
                      color="success"
                    />
                  ))}
                  {deployedPlacement.unmappedGatewayUuids.map((uuid) => (
                    <Chip
                      key={uuid}
                      label="Unmapped"
                      size="small"
                      variant="outlined"
                    />
                  ))}
                </Stack>
              )}
              {(providerData.gateways ?? []).length === 0 && (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Not deployed
                  </Typography>
                  {onGoToDeploymentTab && (
                    <Link
                      component="button"
                      variant="body2"
                      onClick={onGoToDeploymentTab}
                      sx={{ width: "fit-content" }}
                    >
                      Deploy to a gateway
                    </Link>
                  )}
                </>
              )}
            </Stack>
          </Card>
        </Grid>
      </Grid>
      {/* Invoke URLs & API Key section */}
      {orgName && providerId && (
        <InvokeEndpoints
          endpoints={endpoints}
          onGenerateApiKey={handleGenerateApiKey}
          isGeneratingApiKey={createApiKey.isPending || rotateApiKey.isPending}
          apiKeyError={apiKeyError}
          generatedApiKey={generatedApiKey}
          onClearApiKeyError={() => setApiKeyError(null)}
          emptyMessage="No invoke URLs available. Deploy this provider to an AI gateway to see invoke URLs and generate API keys."
        />
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
            slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
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
                isUpdating ||
                !orgName ||
                !providerId
              }
            >
              {isUpdating ? "Saving..." : "Save"}
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
                  <SwaggerSpecViewer
                    {...(swaggerSource.type === "url"
                      ? { url: swaggerSource.value }
                      : { spec: swaggerSource.value as Record<string, unknown> })}
                    docExpansion="list"
                    hideInfoSection
                    hideServers
                    hideAuthorizeButton
                    hideOperationHeader
                  />
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
