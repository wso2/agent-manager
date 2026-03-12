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

import React, { Suspense, useCallback, useState } from "react";
import "swagger-ui-react/swagger-ui.css";

const SwaggerUI = React.lazy(() => import("swagger-ui-react"));
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  Grid,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Download } from "@wso2/oxygen-ui-icons-react";
import type { LLMProviderResponse } from "@agent-management-platform/types";

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
  isLoading?: boolean;
  error?: Error | null;
};

export function LLMProviderOverviewTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
  error: providerError = null,
}: LLMProviderOverviewTabProps) {
  const [isDownloading, setIsDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const handleDownload = useCallback(async () => {
    if (!openapiSpecUrl) return;
    setIsDownloading(true);
    setDownloadError(null);
    try {
      const res = await fetch(openapiSpecUrl);
      if (!res.ok) {
        throw new Error(`Failed to download spec: ${res.status} ${res.statusText}`);
      }
      const text = await res.text();
      const ext = openapiSpecUrl.endsWith(".json") ? "json" : "yaml";
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
  }, [openapiSpecUrl]);

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
    <Stack spacing={2}>
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
                  sx={{ fontFamily: "monospace" }}
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
                    fontFamily: "monospace",
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
      <Divider />
      {/* OpenAPI Resources section */}
      {openapiSpecUrl && (
        <Stack spacing={1.5} sx={{ mt: 3 }}>
          <Typography
            variant="subtitle2"
            color="text.secondary"
            sx={{ fontWeight: 600 }}
          >
            OpenAPI Resources
          </Typography>
          {downloadError && (
            <Alert
              severity="error"
              onClose={() => setDownloadError(null)}
              sx={{ width: "100%" }}
            >
              {downloadError}
            </Alert>
          )}
          <Stack direction="row" spacing={1} alignItems="center">
            <TextField
              size="small"
              fullWidth
              value={openapiSpecUrl}
              slotProps={{ input: { readOnly: true } }}
              sx={{
                "& .MuiInputBase-input": {
                  fontFamily: "monospace",
                  fontSize: "0.875rem",
                },
              }}
            />
            <Button
              variant="outlined"
              size="medium"
              startIcon={<Download size={16} />}
              onClick={handleDownload}
              disabled={isDownloading}
            >
              {isDownloading ? "Downloading..." : "Download"}
            </Button>
          </Stack>
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
              "&.hide-info-section .swagger-ui .info":
                { display: "none !important" },
              "&.hide-servers .swagger-ui .servers, &.hide-servers .swagger-ui .schemes":
                { display: "none !important" },
              "&.hide-authorize .swagger-ui .auth-wrapper":
                { display: "none !important" },
              "&.hide-tag-headers .swagger-ui .opblock-tag-section":
                { display: "none !important" },
              "&.hide-operation-header .swagger-ui .opblock-section-header":
                { display: "none !important" },
              "&.hide-scheme-container .swagger-ui .scheme-container":
                { display: "none !important" },
              "&.hide-models .swagger-ui .models":
                { display: "none !important" },
            }}
          >
            <SwaggerUI
              url={openapiSpecUrl}
              layout="BaseLayout"
              docExpansion="list"
              plugins={[swaggerHideInfoAndServersPlugin]}
            />
          </Box>
          </Suspense>
        </Stack>
      )}
      {!openapiSpecUrl && (
        <Alert severity="info" sx={{ mt: 3 }}>
          No OpenAPI specification is available for this provider&apos;s
          template.
        </Alert>
      )}
    </Stack>
  );
}
