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

import { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  FormControl,
  FormLabel,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Copy, Key } from "@wso2/oxygen-ui-icons-react";

/** Environment an invoke endpoint is exposed through. */
export interface InvokeEndpointEnvironment {
  id?: string;
  name?: string;
  displayName?: string;
  isProduction?: boolean;
}

/** A single gateway invoke URL, optionally tagged with its environment. */
export interface InvokeEndpoint {
  /** Stable id of the gateway backing this endpoint. */
  gatewayId: string;
  /** Fully-built invoke URL for this gateway. */
  url: string;
  /** Gateway display name (falls back to name, then id). */
  displayName?: string;
  name?: string;
  /** Environment this gateway belongs to. Used to group endpoints. */
  environment?: InvokeEndpointEnvironment;
}

export interface InvokeEndpointsProps {
  /** All invoke URLs to show, one per deployed gateway. Grouped by environment. */
  endpoints: InvokeEndpoint[];
  /**
   * API key generation. The key is scoped to the parent resource (proxy /
   * provider), not to an individual gateway, so it is shown once.
   */
  onGenerateApiKey: () => void;
  isGeneratingApiKey: boolean;
  apiKeyError: string | null;
  generatedApiKey: string | null;
  onClearApiKeyError: () => void;
  /** Message shown when there are no endpoints (resource not yet deployed). */
  emptyMessage?: string;
  /** Hides API key generation, for users who can't write the resource. */
  readOnly?: boolean;
}

interface EndpointGroup {
  key: string;
  environment?: InvokeEndpointEnvironment;
  endpoints: InvokeEndpoint[];
}

const UNASSIGNED_KEY = "__unassigned__";

function copyToClipboard(value: string): Promise<void> {
  return navigator.clipboard.writeText(value);
}

/**
 * Shows the invoke URLs of a deployed resource (MCP proxy / LLM provider)
 * grouped by environment — one read-only, copyable URL per gateway — followed
 * by a single API key generator for the whole resource.
 */
export function InvokeEndpoints({
  endpoints,
  onGenerateApiKey,
  isGeneratingApiKey,
  apiKeyError,
  generatedApiKey,
  onClearApiKeyError,
  emptyMessage = "No invoke URLs available. Deploy to an AI gateway to see invoke URLs and generate API keys.",
  readOnly = false,
}: InvokeEndpointsProps) {
  const theme = useTheme();
  const [copiedUrlId, setCopiedUrlId] = useState<string | null>(null);
  const [apiKeyCopied, setApiKeyCopied] = useState(false);

  const groups = useMemo<EndpointGroup[]>(() => {
    const map = new Map<string, EndpointGroup>();
    for (const endpoint of endpoints) {
      const env = endpoint.environment;
      const key = env?.id || env?.name || UNASSIGNED_KEY;
      const existing = map.get(key);
      if (existing) {
        existing.endpoints.push(endpoint);
      } else {
        map.set(key, { key, environment: env, endpoints: [endpoint] });
      }
    }
    // Production environments first, then alphabetical; unassigned last.
    return Array.from(map.values()).sort((a, b) => {
      if (a.key === UNASSIGNED_KEY) return 1;
      if (b.key === UNASSIGNED_KEY) return -1;
      const aProd = a.environment?.isProduction ? 0 : 1;
      const bProd = b.environment?.isProduction ? 0 : 1;
      if (aProd !== bProd) return aProd - bProd;
      const aName = a.environment?.displayName || a.environment?.name || "";
      const bName = b.environment?.displayName || b.environment?.name || "";
      return aName.localeCompare(bName);
    });
  }, [endpoints]);

  const handleCopyUrl = useCallback(async (endpoint: InvokeEndpoint) => {
    if (!endpoint.url) return;
    try {
      await copyToClipboard(endpoint.url);
      setCopiedUrlId(endpoint.gatewayId);
      setTimeout(() => setCopiedUrlId(null), 2000);
    } catch {
      // Silently fail — clipboard access can be denied.
    }
  }, []);

  const handleCopyApiKey = useCallback(async () => {
    if (!generatedApiKey) return;
    try {
      await copyToClipboard(generatedApiKey);
      setApiKeyCopied(true);
      setTimeout(() => setApiKeyCopied(false), 2000);
    } catch {
      // Silently fail.
    }
  }, [generatedApiKey]);

  const monospaceInputSx = {
    "& .MuiInputBase-input": {
      fontFamily: "monospace",
      fontSize: theme.typography.body2?.fontSize,
      wordBreak: "break-all" as const,
    },
  };

  return (
    <Stack spacing={2.5} sx={{ width: "100%" }}>
      <Typography
        variant="subtitle2"
        color="text.secondary"
        sx={{ fontWeight: 600 }}
      >
        Invoke URL & API Key
      </Typography>

      {endpoints.length === 0 ? (
        <Alert severity="info">{emptyMessage}</Alert>
      ) : (
        <Stack spacing={2}>
            {groups.map((group) => {
              const envName =
                group.environment?.displayName || group.environment?.name;
              const envLabel = envName
                ? /environment/i.test(envName)
                  ? envName
                  : `${envName} Environment`
                : "Unassigned";
              return (
                <Card key={group.key} variant="outlined" sx={{ p: 2 }}>
                  <Stack spacing={1.5}>
                    <Stack direction="row" alignItems="center" gap={1}>
                      <Typography variant="body2" sx={{ fontWeight: 600 }}>
                        {envLabel}
                      </Typography>
                      {group.environment?.isProduction && (
                        <Chip
                          size="small"
                          variant="outlined"
                          color="success"
                          label="Production"
                        />
                      )}
                    </Stack>
                    {group.endpoints.map((endpoint) => (
                      <FormControl
                        key={endpoint.gatewayId}
                        fullWidth
                        size="small"
                      >
                        <FormLabel sx={{ mb: 0.5 }}>
                          {endpoint.displayName ||
                            endpoint.name ||
                            endpoint.gatewayId}
                        </FormLabel>
                        <TextField
                          size="small"
                          fullWidth
                          value={endpoint.url}
                          slotProps={{
                            input: {
                              readOnly: true,
                              endAdornment: (
                                <InputAdornment position="end">
                                  <Tooltip
                                    title={
                                      copiedUrlId === endpoint.gatewayId
                                        ? "Copied!"
                                        : "Copy"
                                    }
                                  >
                                    <IconButton
                                      size="small"
                                      onClick={() => handleCopyUrl(endpoint)}
                                      aria-label="Copy Invoke URL"
                                    >
                                      <Copy size={16} />
                                    </IconButton>
                                  </Tooltip>
                                </InputAdornment>
                              ),
                            },
                          }}
                          sx={monospaceInputSx}
                        />
                      </FormControl>
                    ))}
                  </Stack>
                </Card>
              );
            })}
            {!readOnly && <Divider />}

            {!readOnly && (
            <Card variant="outlined" sx={{ p: 2 }}>
              <Stack spacing={1.5}>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    Generate API Key
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    One API key works across all environments above.
                  </Typography>
                </Box>
                <Box>
                  <Button
                    variant="outlined"
                    size="medium"
                    startIcon={<Key size={16} />}
                    onClick={onGenerateApiKey}
                    disabled={isGeneratingApiKey}
                  >
                    {isGeneratingApiKey ? "Generating..." : "Generate API Key"}
                  </Button>
                </Box>
                {apiKeyError && (
                  <Alert severity="error" onClose={onClearApiKeyError}>
                    {apiKeyError}
                  </Alert>
                )}
                {generatedApiKey && (
                  <Alert
                    severity="success"
                    sx={{ "& .MuiAlert-message": { flexGrow: 1 } }}
                  >
                    <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                      API Key Generated
                    </Typography>
                    <Typography variant="body2" sx={{ mb: 1 }}>
                      Copy this API key now. It will not be shown again.
                    </Typography>
                    <Stack
                      direction="row"
                      spacing={1}
                      flexGrow={1}
                      alignItems="center"
                    >
                      <TextField
                        size="small"
                        fullWidth
                        value={generatedApiKey}
                        slotProps={{
                          input: {
                            readOnly: true,
                            endAdornment: (
                              <InputAdornment position="end">
                                <Tooltip
                                  title={apiKeyCopied ? "Copied!" : "Copy"}
                                >
                                  <IconButton
                                    size="small"
                                    onClick={handleCopyApiKey}
                                    aria-label="Copy API Key"
                                  >
                                    <Copy size={16} />
                                  </IconButton>
                                </Tooltip>
                              </InputAdornment>
                            ),
                          },
                        }}
                        sx={monospaceInputSx}
                      />
                    </Stack>
                  </Alert>
                )}
              </Stack>
            </Card>
            )}
          </Stack>
      )}
    </Stack>
  );
}
