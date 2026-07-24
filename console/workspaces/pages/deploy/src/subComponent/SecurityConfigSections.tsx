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
  useGetAgent,
  useListEnvironmentIdentityProviders,
} from "@agent-management-platform/api-client";
import type {
  ConfigurationResponse,
  UpdateAgentDeploySettingsRequest,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Autocomplete,
  Box,
  Checkbox,
  Chip,
  Collapse,
  Form,
  FormControl,
  FormControlLabel,
  FormHelperText,
  FormLabel,
  Grid,
  MenuItem,
  Radio,
  RadioGroup,
  Select,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useState,
} from "react";

const DEFAULT_RESILIENCE_TIMEOUT_SECONDS = 30;
const MIN_RESILIENCE_TIMEOUT_SECONDS = 1;
const MAX_RESILIENCE_TIMEOUT_SECONDS = 3600;

type TimeoutUnit = "seconds" | "minutes";

// Displays a persisted seconds value in the larger unit when it divides evenly, purely so the
// input doesn't show "600" when "10 minutes" reads more naturally — the persisted value is
// always seconds regardless of which unit the user views/edits it in.
function toDurationAndUnit(seconds: number): { duration: string; unit: TimeoutUnit } {
  if (seconds >= 60 && seconds % 60 === 0) {
    return { duration: String(seconds / 60), unit: "minutes" };
  }
  return { duration: String(seconds), unit: "seconds" };
}

export interface SecurityConfigHandle {
  // Returns true when the current selection is valid (e.g. OAuth has an issuer).
  validate: () => boolean;
  // Builds the security portion of the deploy-settings body to merge into the drawer's Apply.
  buildBody: () => Partial<UpdateAgentDeploySettingsRequest>;
}

interface SecurityConfigSectionsProps {
  orgName: string;
  projName: string;
  agentName: string;
  environment: string;
  open: boolean;
  disabled?: boolean;
  // Per-environment configuration used to seed CORS + Endpoint Authentication. Must be the
  // env-scoped GET /configurations response (NOT GetAgent, which returns only the lowest
  // environment's values and would paint every env the same).
  configurations?: ConfigurationResponse;
  // Fires whenever validity changes so the parent can enable/disable its Apply button reactively
  // (the imperative validate() alone can't drive re-renders).
  onValidityChange?: (valid: boolean) => void;
}

// CORS + Endpoint Authentication form sections, extracted so they can be composed into the unified
// "Configurations and Secrets" drawer. Owns its own draft state; the parent collects the payload
// via the imperative handle on Apply.
export const SecurityConfigSections = forwardRef<SecurityConfigHandle, SecurityConfigSectionsProps>(
  (
    { orgName, projName, agentName, environment, open, disabled, configurations, onValidityChange },
    ref,
  ) => {
    // GetAgent is used only for the env-independent agent type; CORS + Auth are seeded from the
    // env-scoped `configurations` prop below.
    const { data: agent } = useGetAgent({ orgName, projName, agentName });
    const isApiAgent = agent?.agentType?.type === "agent-api";

    const { data: idpResp } = useListEnvironmentIdentityProviders({
      orgName,
      environmentId: open && isApiAgent ? environment : undefined,
    });
    const identityProviderOptions = useMemo(
      () => (idpResp?.list ?? []).map((p) => p.name),
      [idpResp],
    );
    const hasIdentityProviders = identityProviderOptions.length > 0;

    const [authMode, setAuthMode] = useState<"none" | "apikey" | "oauth">("apikey");
    const [oauthIssuers, setOauthIssuers] = useState<string[]>([]);
    const [oauthAudiences, setOauthAudiences] = useState<string[]>([]);
    const [oauthHeaderName, setOauthHeaderName] = useState<string>("Authorization");
    const [oauthHeaderPrefix, setOauthHeaderPrefix] = useState<string>("Bearer");
    const [oauthForwardToken, setOauthForwardToken] = useState<boolean>(true);

    const [corsEnabled, setCorsEnabled] = useState(false);
    const [corsAllowAll, setCorsAllowAll] = useState(true);
    const [corsOrigins, setCorsOrigins] = useState<string[]>(["*"]);
    const [corsMethods, setCorsMethods] = useState<string[]>([
      "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS",
    ]);
    const [corsHeaders, setCorsHeaders] = useState<string[]>([
      "authorization", "Content-Type", "Origin", "X-API-Key",
    ]);
    const [corsAllowCredentials, setCorsAllowCredentials] = useState(false);
    const [timeoutDuration, setTimeoutDuration] = useState("30");
    const [timeoutUnit, setTimeoutUnit] = useState<TimeoutUnit>("seconds");

    useEffect(() => {
      if (!open) return;
      const seconds =
        configurations?.resilienceTimeoutSeconds ?? DEFAULT_RESILIENCE_TIMEOUT_SECONDS;
      const { duration, unit } = toDurationAndUnit(seconds);
      setTimeoutDuration(duration);
      setTimeoutUnit(unit);
    }, [open, configurations?.resilienceTimeoutSeconds]);

    // Seed CORS from the env-scoped config on open; reset to defaults when no persisted config
    // exists so stale edits from a previous open don't leak across reopens.
    useEffect(() => {
      if (!open) return;
      const cors = configurations?.corsConfig;
      if (!cors) {
        setCorsEnabled(false);
        setCorsAllowAll(true);
        setCorsOrigins(["*"]);
        setCorsMethods(["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]);
        setCorsHeaders(["authorization", "Content-Type", "Origin", "X-API-Key"]);
        setCorsAllowCredentials(false);
        return;
      }
      if (cors.enabled !== undefined) setCorsEnabled(cors.enabled);
      if (cors.allowOrigin !== undefined) {
        const isWildcard = cors.allowOrigin.length === 1 && cors.allowOrigin[0] === "*";
        setCorsAllowAll(isWildcard);
        setCorsOrigins(cors.allowOrigin);
      }
      if (cors.allowMethods !== undefined) setCorsMethods(cors.allowMethods);
      if (cors.allowHeaders !== undefined) setCorsHeaders(cors.allowHeaders);
      if (cors.allowCredentials !== undefined) setCorsAllowCredentials(cors.allowCredentials);
    }, [open, configurations?.corsConfig]);

    // Seed Endpoint Authentication from the env-scoped config on open.
    useEffect(() => {
      if (!open) return;
      const cfg = configurations;
      if (cfg?.enableOAuthSecurity) {
        setAuthMode("oauth");
      } else if (cfg?.enableApiKeySecurity) {
        setAuthMode("apikey");
      } else if (cfg?.enableApiKeySecurity === false) {
        setAuthMode("none");
      } else {
        setAuthMode("apikey");
      }
      const oauth = cfg?.oauthConfig;
      setOauthIssuers(oauth?.issuers ?? []);
      setOauthAudiences(oauth?.audiences ?? []);
      setOauthHeaderName(oauth?.headerName || "Authorization");
      setOauthHeaderPrefix(oauth?.authHeaderPrefix || "Bearer");
      setOauthForwardToken(oauth?.forwardToken ?? true);
      // Seed on open from the specific auth fields; depending on the whole `configurations`
      // object would re-seed on unrelated config changes.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
      open,
      configurations?.enableApiKeySecurity,
      configurations?.enableOAuthSecurity,
      configurations?.oauthConfig,
    ]);

    const hasWildcardOrigin = corsAllowAll || corsOrigins.includes("*");
    const oauthInvalid = isApiAgent && authMode === "oauth" && oauthIssuers.length === 0;

    const resilienceTimeoutSeconds = timeoutUnit === "minutes"
      ? Number(timeoutDuration) * 60
      : Number(timeoutDuration);
    const timeoutInvalid = isApiAgent && (
      timeoutDuration.trim() === ""
      || !Number.isFinite(resilienceTimeoutSeconds)
      || resilienceTimeoutSeconds < MIN_RESILIENCE_TIMEOUT_SECONDS
      || resilienceTimeoutSeconds > MAX_RESILIENCE_TIMEOUT_SECONDS
    );


    const handleTimeoutBlur = () => {
      if (!timeoutInvalid) return;
      setTimeoutDuration(String(DEFAULT_RESILIENCE_TIMEOUT_SECONDS));
      setTimeoutUnit("seconds");
    };

    useEffect(() => {
      onValidityChange?.(!oauthInvalid && !timeoutInvalid);
    }, [oauthInvalid, timeoutInvalid, onValidityChange]);

    useImperativeHandle(
      ref,
      () => ({
        validate: () => !oauthInvalid && !timeoutInvalid,
        buildBody: () => {
          if (!isApiAgent) return {};
          return {
            enableApiKeySecurity: authMode === "apikey",
            enableOAuthSecurity: authMode === "oauth",
            ...(authMode === "oauth" && {
              oauthConfig: {
                issuers: oauthIssuers,
                audiences: oauthAudiences,
                headerName: oauthHeaderName.trim() || "Authorization",
                authHeaderPrefix: oauthHeaderPrefix.trim() || "Bearer",
                forwardToken: oauthForwardToken,
              },
            }),
            corsConfig: {
              enabled: corsEnabled,
              allowOrigin: hasWildcardOrigin ? ["*"] : corsOrigins,
              allowMethods: corsMethods,
              allowHeaders: corsHeaders,
              allowCredentials: hasWildcardOrigin ? false : corsAllowCredentials,
            },
            resilienceTimeoutSeconds: timeoutInvalid
              ? DEFAULT_RESILIENCE_TIMEOUT_SECONDS
              : resilienceTimeoutSeconds,
          };
        },
      }),
      [
        isApiAgent, oauthInvalid, authMode, oauthIssuers, oauthAudiences, oauthHeaderName,
        oauthHeaderPrefix, oauthForwardToken, corsEnabled, corsOrigins, corsMethods,
        corsHeaders, corsAllowCredentials, hasWildcardOrigin, timeoutInvalid,
        resilienceTimeoutSeconds,
      ],
    );

    if (!isApiAgent) return null;

    return (
      <>
        {/* ── CORS ─────────────────────────────────────────────────── */}
        <Form.Section>
          <Form.Header>CORS Configuration</Form.Header>
          <Form.Subheader>
            Control which origins, methods, and headers may access this endpoint.
          </Form.Subheader>
          <Form.Stack spacing={1}>
            <FormControlLabel
              control={
                <Switch
                  checked={corsEnabled}
                  onChange={(_, checked) => setCorsEnabled(checked)}
                  disabled={disabled}
                />
              }
              label="Enable CORS"
            />
            <Collapse in={corsEnabled}>
              <Accordion
                disableGutters
                elevation={0}
                sx={{
                  mt: 1,
                  border: "1px solid",
                  borderColor: "divider",
                  borderRadius: 1,
                  "&:before": { display: "none" },
                }}
              >
                <AccordionSummary expandIcon={<ChevronDown size={16} />}>
                  <Typography variant="body2">Advanced</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Form.Stack spacing={2}>
                    <Box display="flex" gap={2} alignItems="center">
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={corsAllowAll}
                            onChange={(_, checked) => {
                              setCorsAllowAll(checked);
                              if (checked) {
                                setCorsAllowCredentials(false);
                                setCorsOrigins(["*"]);
                              } else {
                                setCorsOrigins((prev) => prev.filter((o) => o !== "*"));
                              }
                            }}
                            disabled={disabled}
                          />
                        }
                        label="Allow all origins"
                      />
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={corsAllowCredentials}
                            onChange={(_, checked) => setCorsAllowCredentials(checked)}
                            disabled={disabled || hasWildcardOrigin}
                          />
                        }
                        label="Allow credentials"
                      />
                    </Box>
                    {!corsAllowAll && (
                      <FormControl fullWidth>
                        <FormLabel>Allowed origins</FormLabel>
                        <Autocomplete
                          multiple
                          freeSolo
                          options={[]}
                          value={corsOrigins}
                          onChange={(_, v) => setCorsOrigins(v as string[])}
                          renderTags={(vals, getTagProps) =>
                            vals.map((opt, i) => (
                              <Chip
                                label={opt as string}
                                size="small"
                                {...getTagProps({ index: i })}
                                key={opt as string}
                              />
                            ))
                          }
                          renderInput={(params) => (
                            <TextField {...params} size="small" placeholder="Add origin and press Enter" />
                          )}
                        />
                      </FormControl>
                    )}
                    <FormControl fullWidth>
                      <FormLabel>Allowed methods</FormLabel>
                      <Autocomplete
                        multiple
                        freeSolo
                        options={[]}
                        value={corsMethods}
                        onChange={(_, v) => setCorsMethods(v as string[])}
                        renderTags={(vals, getTagProps) =>
                          vals.map((opt, i) => (
                            <Chip
                              label={opt as string}
                              size="small"
                              {...getTagProps({ index: i })}
                              key={opt as string}
                            />
                          ))
                        }
                        renderInput={(params) => (
                          <TextField {...params} size="small" placeholder="Add method and press Enter" />
                        )}
                      />
                    </FormControl>
                    <FormControl fullWidth>
                      <FormLabel>Allowed headers</FormLabel>
                      <Autocomplete
                        multiple
                        freeSolo
                        options={[]}
                        value={corsHeaders}
                        onChange={(_, v) => setCorsHeaders(v as string[])}
                        renderTags={(vals, getTagProps) =>
                          vals.map((opt, i) => (
                            <Chip
                              label={opt as string}
                              size="small"
                              {...getTagProps({ index: i })}
                              key={opt as string}
                            />
                          ))
                        }
                        renderInput={(params) => (
                          <TextField {...params} size="small" placeholder="Add header and press Enter" />
                        )}
                      />
                    </FormControl>
                  </Form.Stack>
                </AccordionDetails>
              </Accordion>
            </Collapse>
          </Form.Stack>
        </Form.Section>

        {/* ── Endpoint Authentication ──────────────────────────────── */}
        <Form.Section>
          <Form.Header>Endpoint Authentication</Form.Header>
          <Form.Subheader>
            Choose how callers authenticate against this agent endpoint.
          </Form.Subheader>
          <Form.Stack spacing={1}>
            <FormControl>
              <RadioGroup
                value={authMode}
                onChange={(_, value) => {
                  const mode = value as "none" | "apikey" | "oauth";
                  setAuthMode(mode);
                  if (mode === "oauth" && oauthIssuers.length === 0 && hasIdentityProviders) {
                    setOauthIssuers(identityProviderOptions);
                  }
                }}
              >
                <FormControlLabel
                  value="none"
                  control={<Radio disabled={disabled} />}
                  label="No authentication"
                />
                <FormControlLabel
                  value="apikey"
                  control={<Radio disabled={disabled} />}
                  label="API key"
                />
                <FormControlLabel
                  value="oauth"
                  control={<Radio disabled={disabled || !hasIdentityProviders} />}
                  label={
                    hasIdentityProviders
                      ? "OAuth"
                      : "OAuth — configure an identity provider first"
                  }
                />
              </RadioGroup>
            </FormControl>

            <Collapse in={authMode === "apikey"}>
              <FormControl fullWidth sx={{ mt: 1 }}>
                <FormLabel>Header</FormLabel>
                <TextField value="X-API-Key" size="small" fullWidth disabled />
              </FormControl>
            </Collapse>

            <Collapse in={authMode === "oauth"}>
              <Form.Stack spacing={2} sx={{ mt: 1 }}>
                {!hasIdentityProviders && (
                  <Alert severity="warning">
                    <Typography variant="caption">
                      No identity providers for this environment. Add one under Security &rarr;
                      Identity Providers first.
                    </Typography>
                  </Alert>
                )}
                <FormControl fullWidth error={oauthInvalid}>
                  <FormLabel required>Identity Providers</FormLabel>
                  <Autocomplete
                    multiple
                    options={identityProviderOptions}
                    value={oauthIssuers}
                    onChange={(_, v) => setOauthIssuers(v as string[])}
                    disabled={disabled || !hasIdentityProviders}
                    renderTags={(vals, getTagProps) =>
                      vals.map((opt, i) => (
                        <Chip
                          label={opt as string}
                          size="small"
                          {...getTagProps({ index: i })}
                          key={opt as string}
                        />
                      ))
                    }
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        size="small"
                        error={oauthInvalid}
                        placeholder={oauthIssuers.length === 0 ? "Select identity providers" : ""}
                      />
                    )}
                  />
                  {oauthInvalid && (
                    <FormHelperText>Select at least one identity provider.</FormHelperText>
                  )}
                </FormControl>

                <FormControl fullWidth>
                  <FormLabel>Audiences</FormLabel>
                  <Autocomplete
                    multiple
                    freeSolo
                    options={[]}
                    value={oauthAudiences}
                    onChange={(_, v) => setOauthAudiences(v as string[])}
                    disabled={disabled}
                    renderTags={(vals, getTagProps) =>
                      vals.map((opt, i) => (
                        <Chip
                          label={opt as string}
                          size="small"
                          {...getTagProps({ index: i })}
                          key={opt as string}
                        />
                      ))
                    }
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        size="small"
                        placeholder={
                          oauthAudiences.length === 0 ? "Add accepted audience (aud) values" : ""
                        }
                      />
                    )}
                  />
                  <FormHelperText>
                    Accepted token audiences (aud claim). Leave empty to disable audience
                    validation.
                  </FormHelperText>
                </FormControl>

                <Box display="flex" gap={2}>
                  <FormControl fullWidth>
                    <FormLabel>Header name</FormLabel>
                    <TextField
                      size="small"
                      fullWidth
                      value={oauthHeaderName}
                      disabled={disabled}
                      onChange={(e) => setOauthHeaderName(e.target.value)}
                      placeholder="Authorization"
                    />
                  </FormControl>
                  <FormControl fullWidth>
                    <FormLabel>Auth header prefix</FormLabel>
                    <TextField
                      size="small"
                      fullWidth
                      value={oauthHeaderPrefix}
                      disabled={disabled}
                      onChange={(e) => setOauthHeaderPrefix(e.target.value)}
                      placeholder="Bearer"
                    />
                  </FormControl>
                </Box>

                <FormControlLabel
                  control={
                    <Switch
                      checked={oauthForwardToken}
                      onChange={(_, checked) => setOauthForwardToken(checked)}
                      disabled={disabled}
                    />
                  }
                  label="Forward token to upstream"
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: -1 }}>
                  Forward the token header to the upstream service after validation. Disable to
                  strip it before proxying.
                </Typography>
              </Form.Stack>
            </Collapse>
          </Form.Stack>
        </Form.Section>
        <Form.Section>
          <Form.Header>Gateway Timeout</Form.Header>
          <Form.Subheader>
            Max duration the gateway keeps a response open between this agent and the client
          </Form.Subheader>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6 }}>
              <FormControl fullWidth size="small" error={timeoutInvalid}>
                <FormLabel>Duration</FormLabel>
                <TextField
                  size="small"
                  type="number"
                  placeholder="e.g. 30"
                  value={timeoutDuration}
                  onChange={(e) => setTimeoutDuration(e.target.value)}
                  onBlur={handleTimeoutBlur}
                  disabled={disabled}
                  error={timeoutInvalid}
                  helperText={
                    timeoutInvalid
                      ? `Enter a duration between ${MIN_RESILIENCE_TIMEOUT_SECONDS}s and `
                        + `${MAX_RESILIENCE_TIMEOUT_SECONDS / 60}m`
                      : undefined
                  }
                  slotProps={{ input: { inputProps: { min: 1, step: 1 } } }}
                />
              </FormControl>
            </Grid>
            <Grid size={{ xs: 12, sm: 6 }}>
              <FormControl fullWidth size="small">
                <FormLabel>Unit</FormLabel>
                <Select
                  size="small"
                  value={timeoutUnit}
                  onChange={(e) => setTimeoutUnit(e.target.value as TimeoutUnit)}
                  disabled={disabled}
                >
                  <MenuItem value="seconds">Seconds</MenuItem>
                  <MenuItem value="minutes">Minutes</MenuItem>
                </Select>
              </FormControl>
            </Grid>
          </Grid>
        </Form.Section>
      </>
    );
  },
);

SecurityConfigSections.displayName = "SecurityConfigSections";
