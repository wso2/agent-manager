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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CardContent,
  Collapse,
  Form,
  IconButton,
  InputAdornment,
  Skeleton,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Brain, Eye, EyeOff, Lock, Unlock } from "@wso2/oxygen-ui-icons-react";
import {
  addLLMProviderSchema,
  type AddLLMProviderFormValues,
} from "../form/schema";
import { useValidatedForm } from "../hooks/useValidatedForm";
import {
  EnvironmentGatewaySelector,
  getErrorMessage,
  PolicyListSection,
  ResilienceTimeoutFields,
  type PolicySelection as GuardrailSelection,
} from "@agent-management-platform/shared-component";
import { useListGateways } from "@agent-management-platform/api-client";

export type TemplateCard = {
  id: string;
  /**
   * Template handle from the backend (e.g., "openai").
   */
  handle: string;
  /**
   * Human-friendly display name shown in the UI.
   */
  name: string;
  description?: string;
  image?: string;
  hasTemplateUrl?: boolean;
  endpointUrl?: string;
  hasTemplateAuthType?: boolean;
  hasTemplateAuthHeader?: boolean;
  /**
   * Auth type from template metadata (e.g., "api-key").
   */
  authType?: string;
  /**
   * Auth header from template metadata (e.g., "Authorization").
   */
  authHeader?: string;
  /**
   * Value prefix from template metadata (e.g., "Bearer " for bearer tokens).
   */
  authValuePrefix?: string;
};

export type { AddLLMProviderFormValues, GuardrailSelection };

interface AddLLMProviderFormProps {
  orgId: string;
  templates: TemplateCard[];
  isLoadingTemplates: boolean;
  missingParamsMessage?: string | null;
  errorMessage?: string | null;
  isSubmitting?: boolean;
  submitLabel?: string;
  /**
   * Set to false to collapse the Basic Details section to just the Name
   * field, hiding Version / Description / Context path. Their defaults
   * (version `v1.0`, auto-derived context, empty description) are still
   * submitted, so the provider is created with no extra input.
   */
  showAdvancedBasicDetails?: boolean;
  /**
   * Set to false to hide the Guardrails section, e.g. when the form is
   * embedded in a flow that only needs a minimal provider (name, template,
   * connection config).
   */
  showGuardrails?: boolean;
  onCancel: () => void;
  onSubmit: (
    values: AddLLMProviderFormValues,
    guardrails: GuardrailSelection[],
  ) => void;
}

function toContextPath(name: string): string {
  const slug = name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug ? `/${slug}` : "";
}

const INITIAL_FORM_VALUES: AddLLMProviderFormValues = {
  templateId: "",
  displayName: "",
  version: "v1.0",
  description: "",
  context: "",
  upstreamUrl: "",
  apiKey: "",
  resilienceTimeout: "",
  resilienceIdleTimeout: "",
  gatewayIds: [],
};

// Provider template cards keep a minimum width so names never truncate; the grid
// fits as many columns as the container allows and reflows responsively.
const TEMPLATE_GRID_COLUMNS = "repeat(auto-fill, minmax(min(100%, 240px), 1fr))";

export const AddLLMProviderForm: React.FC<AddLLMProviderFormProps> = ({
  orgId,
  templates,
  isLoadingTemplates,
  missingParamsMessage,
  errorMessage,
  isSubmitting,
  submitLabel = "Add provider",
  showAdvancedBasicDetails = true,
  showGuardrails = true,
  onCancel,
  onSubmit,
}) => {
  const [formData, setFormData] =
    useState<AddLLMProviderFormValues>(INITIAL_FORM_VALUES);

  const {
    errors,
    setFieldError,
    validateField,
    lastSubmittedValidationErrors,
    guardSubmit,
  } = useValidatedForm<AddLLMProviderFormValues>(addLLMProviderSchema);

  const sortedTemplates = useMemo(
    () => [...templates].sort((a, b) => a.name.localeCompare(b.name)),
    [templates],
  );

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === formData.templateId) ?? null,
    [formData.templateId, templates],
  );

  // limit: 500 — without it the server's default page size silently truncates
  // the list and the zero-egress warning could fire against a partial view.
  const {
    data: gatewaysData,
    isLoading: isLoadingGateways,
    error: gatewaysFetchError,
  } = useListGateways({ orgName: orgId }, { limit: 500 });

  // Egress-capable only: ingress gateways are not legal LLM placement targets and the
  // server rejects them. No status filter — the server's candidate set is not
  // liveness-filtered either, so filtering here would offer a narrower set than the
  // server accepts and hide a valid choice whenever a gateway is briefly disconnected.
  const gateways = useMemo(
    () =>
      gatewaysData?.gateways.filter(
        (g) => g.gatewayType === "EGRESS" || g.gatewayType === "BOTH",
      ) ?? [],
    [gatewaysData?.gateways],
  );

  const hasTemplateUrl = Boolean(selectedTemplate?.hasTemplateUrl);
  const requiresUpstream = !hasTemplateUrl;
  const requiresApiKey = !!selectedTemplate?.hasTemplateAuthHeader;

  const [guardrails, setGuardrails] = useState<GuardrailSelection[]>([]);
  const [isGatewaySelectionValid, setIsGatewaySelectionValid] = useState(true);
  // True once the selector auto-resolved a single unambiguous environment —
  // nothing to configure, so the whole section (including its title) hides.
  const [isSingleGatewayChoice, setIsSingleGatewayChoice] = useState(false);
  const [endpointEditable, setEndpointEditable] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  // Tracks whether the user has taken manual control of the context path.
  // Set on a direct edit to Context; cleared when they empty it back out,
  // which is the signal to resume auto-deriving from Name.
  const [isContextManuallyEdited, setIsContextManuallyEdited] =
    useState(false);

  const handleAddGuardrail = useCallback((guardrail: GuardrailSelection) => {
    setGuardrails((prev) => {
      if (
        prev.some(
          (g) => g.name === guardrail.name && g.version === guardrail.version,
        )
      )
        return prev;
      return [...prev, guardrail];
    });
  }, []);

  const handleEditGuardrail = useCallback((guardrail: GuardrailSelection) => {
    setGuardrails((prev) =>
      prev.map((g) =>
        g.name === guardrail.name && g.version === guardrail.version
          ? guardrail
          : g,
      ),
    );
  }, []);

  const handleRemoveGuardrail = useCallback((name: string, version: string) => {
    setGuardrails((prev) =>
      prev.filter((g) => !(g.name === name && g.version === version)),
    );
  }, []);

  useEffect(() => {
    if (selectedTemplate) {
      setFormData((prev) => ({
        ...prev,
        upstreamUrl: selectedTemplate.endpointUrl ?? "",
      }));
    }
  }, [selectedTemplate]);

  // Name auto-derives the context path until the user manually edits Context,
  // at which point their input wins and Name edits stop overwriting it.
  // Clearing Context back to empty un-locks it, but deliberately does NOT
  // repopulate it on its own — isContextManuallyEdited is intentionally left
  // out of the dependency array so derivation only fires on the next actual
  // Name edit, not the instant Context is cleared.
  useEffect(() => {
    if (isContextManuallyEdited) {
      return;
    }
    const derived = toContextPath(formData.displayName);
    setFormData((prev) =>
      prev.context === derived ? prev : { ...prev, context: derived },
    );
    setFieldError(
      "context",
      validateField("context", derived, { ...formData, context: derived }),
    );
    // Only run when displayName changes; formData for validation
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formData.displayName]);

  const handleFieldChange = useCallback(
    (field: keyof AddLLMProviderFormValues, value: string | string[]) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value } as AddLLMProviderFormValues;
        const fieldError = validateField(field, next[field], next);
        setFieldError(field, fieldError);
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleContextChange = useCallback(
    (value: string) => {
      setIsContextManuallyEdited(value.trim() !== "");
      handleFieldChange("context", value);
    },
    [handleFieldChange],
  );

  const handleTemplateSelect = useCallback(
    (templateId: string) => {
      setEndpointEditable(false);
      setFormData((prev) => {
        const next: AddLLMProviderFormValues = {
          ...prev,
          templateId,
          upstreamUrl: "",
          apiKey: "",
        };
        const fieldError = validateField("templateId", templateId, next);
        setFieldError("templateId", fieldError);
        // Clear any stale upstream/apiKey errors when switching templates.
        setFieldError("upstreamUrl", undefined);
        setFieldError("apiKey", undefined);
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleSubmit = useCallback(() => {
    let hasHardError = false;

    if (requiresUpstream && !formData.upstreamUrl.trim()) {
      setFieldError("upstreamUrl", "Upstream endpoint is required");
      hasHardError = true;
    }

    if (requiresApiKey && !formData.apiKey?.trim()) {
      setFieldError("apiKey", "API key / credential is required");
      hasHardError = true;
    }

    if (hasHardError) {
      return;
    }

    if (!guardSubmit(formData)) {
      return;
    }

    onSubmit(
      {
        ...formData,
        displayName: formData.displayName.trim(),
        version: formData.version.trim(),
        description: formData.description?.trim() ?? "",
        context: formData.context?.trim() ?? "",
        upstreamUrl: formData.upstreamUrl?.trim() ?? "",
        apiKey: requiresApiKey ? (formData.apiKey?.trim() ?? "") : "",
        resilienceTimeout: formData.resilienceTimeout?.trim() ?? "",
        resilienceIdleTimeout: formData.resilienceIdleTimeout?.trim() ?? "",
        gatewayIds: formData.gatewayIds ?? [],
      },
      guardrails,
    );
  }, [
    formData,
    guardSubmit,
    guardrails,
    onSubmit,
    requiresApiKey,
    requiresUpstream,
    setFieldError,
  ]);

  const submittedErrorsList = useMemo(() => {
    const entries = Object.entries(lastSubmittedValidationErrors).filter(
      ([, msg]) => msg,
    ) as [string, string][];
    return entries.length > 0 ? entries : null;
  }, [lastSubmittedValidationErrors]);

  return (
    <Form.Stack spacing={3}>
      {missingParamsMessage && (
        <Typography color="error" variant="body2">
          {missingParamsMessage}
        </Typography>
      )}
      {/* Template selector */}
      <Form.Section>
        <Form.Subheader>Basic Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.Stack
            direction={{ xs: "column", md: "row" }}
            spacing={2}
            useFlexGap
          >
            <Box sx={{ flex: 2 }}>
              <Form.ElementWrapper label="Name" name="displayName">
                <TextField
                  id="displayName"
                  fullWidth
                  value={formData.displayName}
                  onChange={(e) =>
                    handleFieldChange("displayName", e.target.value)
                  }
                  placeholder="Production OpenAI Provider"
                  error={Boolean(errors.displayName)}
                  helperText={errors.displayName}
                />
              </Form.ElementWrapper>
            </Box>

            {showAdvancedBasicDetails && (
              <Box sx={{ flex: 1 }}>
                <Form.ElementWrapper label="Version" name="version">
                  <TextField
                    id="version"
                    fullWidth
                    value={formData.version}
                    onChange={(e) =>
                      handleFieldChange("version", e.target.value)
                    }
                    placeholder="v1.0"
                    error={Boolean(errors.version)}
                    helperText={errors.version}
                  />
                </Form.ElementWrapper>
              </Box>
            )}
          </Form.Stack>

          {showAdvancedBasicDetails && (
            <>
              <Form.ElementWrapper
                label="Description (optional)"
                name="description"
              >
                <TextField
                  id="description"
                  fullWidth
                  multiline
                  rows={2}
                  value={formData.description ?? ""}
                  onChange={(e) =>
                    handleFieldChange("description", e.target.value)
                  }
                  placeholder="Primary LLM provider for production"
                  error={Boolean(errors.description)}
                  helperText={errors.description}
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper
                label="Context path (optional)"
                name="context"
              >
                <TextField
                  id="context"
                  fullWidth
                  value={formData.context ?? ""}
                  onChange={(e) => handleContextChange(e.target.value)}
                  placeholder="/my-provider"
                  error={Boolean(errors.context)}
                  helperText={
                    errors.context ??
                    "API context path (must start with /, no trailing slash)"
                  }
                />
              </Form.ElementWrapper>
            </>
          )}
        </Form.Stack>
      </Form.Section>

      {isLoadingTemplates && (
        <Box>
          <Skeleton variant="text" width={140} height={20} sx={{ mb: 1.5 }} />
          <Box
            sx={{
              display: "grid",
              gap: 1.5,
              gridTemplateColumns: TEMPLATE_GRID_COLUMNS,
            }}
          >
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} variant="rounded" height={72} />
            ))}
          </Box>
        </Box>
      )}

      <Form.Section>
        <Form.Subheader>Provider Template</Form.Subheader>
        <Box
          sx={{
            mt: 1.5,
            display: "grid",
            gap: 1.5,
            gridTemplateColumns: TEMPLATE_GRID_COLUMNS,
          }}
        >
            {sortedTemplates.map((template) => {
              const isSelected = formData.templateId === template.id;
              return (
                <Form.CardButton
                  key={template.id}
                  selected={isSelected}
                  onClick={() => handleTemplateSelect(template.id)}
                >
                  <CardContent>
                    <Form.Stack
                      direction="row"
                      spacing={2}
                      pt={0.5}
                      alignItems="center"
                    >
                      {template.image ? (
                        <Box
                          component="img"
                          src={template.image}
                          alt={template.name}
                          sx={{
                            width: 28,
                            height: 28,
                            objectFit: "contain",
                            backgroundColor: "common.white",
                            borderRadius: "20%",
                            flexShrink: 0,
                          }}
                        />
                      ) : (
                        <Brain size={24} />
                      )}
                      <Form.Stack spacing={0.25}>
                        <Typography variant="subtitle2" noWrap>
                          {template.name}
                        </Typography>
                        {template.description && (
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            noWrap
                          >
                            {template.description}
                          </Typography>
                        )}
                      </Form.Stack>
                    </Form.Stack>
                  </CardContent>
                </Form.CardButton>
              );
            })}
            {!sortedTemplates.length && !isLoadingTemplates && (
              <Typography variant="body2" color="text.secondary">
                No provider templates available for this organization.
              </Typography>
            )}
        </Box>
      </Form.Section>
      <Collapse in={!!formData.templateId}>
        <Form.Section>
          <Form.Subheader>Runtime Configuration</Form.Subheader>
          <Form.Stack spacing={2}>
            <Form.ElementWrapper label="Endpoint URL" name="upstreamUrl">
              <TextField
                id="upstreamUrl"
                fullWidth
                value={formData.upstreamUrl ?? ""}
                onChange={(e) => handleFieldChange("upstreamUrl", e.target.value)}
                placeholder="https://api.openai.com/v1"
                error={Boolean(errors.upstreamUrl)}
                helperText={errors.upstreamUrl}
                // Template-provided endpoints are locked (read-only) until the
                // user opts to override them via the adornment toggle.
                slotProps={{
                  input: {
                    readOnly: hasTemplateUrl && !endpointEditable,
                    endAdornment: hasTemplateUrl ? (
                      <InputAdornment position="end">
                        <Tooltip
                          title={
                            endpointEditable
                              ? "Lock endpoint"
                              : "Unlock to override endpoint"
                          }
                        >
                          <IconButton
                            size="small"
                            onClick={() => setEndpointEditable((v) => !v)}
                          >
                            {endpointEditable ? (
                              <Unlock size={14} />
                            ) : (
                              <Lock size={14} />
                            )}
                          </IconButton>
                        </Tooltip>
                      </InputAdornment>
                    ) : undefined,
                  },
                }}
              />
            </Form.ElementWrapper>

            <Form.ElementWrapper
              label="API key / Credential"
              name="apiKey"
            >
              <TextField
                id="apiKey"
                fullWidth
                type={showApiKey ? "text" : "password"}
                value={formData.apiKey}
                onChange={(e) => handleFieldChange("apiKey", e.target.value)}
                placeholder="Enter your API key"
                error={Boolean(errors.apiKey)}
                helperText={errors.apiKey}
                slotProps={{
                  input: {
                    endAdornment: (
                      <InputAdornment position="end">
                        <Tooltip title={showApiKey ? "Hide" : "Show"}>
                          <IconButton
                            size="small"
                            edge="end"
                            aria-label={
                              showApiKey ? "Hide API key" : "Show API key"
                            }
                            onClick={() => setShowApiKey((v) => !v)}
                          >
                            {showApiKey ? (
                              <EyeOff size={16} />
                            ) : (
                              <Eye size={16} />
                            )}
                          </IconButton>
                        </Tooltip>
                      </InputAdornment>
                    ),
                  },
                }}
              />
            </Form.ElementWrapper>
          </Form.Stack>
          <Form.Stack>
            <ResilienceTimeoutFields
              requestTimeout={formData.resilienceTimeout ?? ""}
              onRequestTimeoutChange={(value) => handleFieldChange("resilienceTimeout", value)}
              requestTimeoutError={errors.resilienceTimeout}
              idleTimeout={formData.resilienceIdleTimeout ?? ""}
              onIdleTimeoutChange={(value) => handleFieldChange("resilienceIdleTimeout", value)}
              idleTimeoutError={errors.resilienceIdleTimeout}
            />
          </Form.Stack>
        </Form.Section>

        
      </Collapse>
      {/* Guardrails */}
      {showGuardrails && (
        <Collapse in={!!formData.templateId}>
          <PolicyListSection
            title="Guardrails"
            description="Add safety policies to enforce consistent protections."
            addButtonLabel="Add Guardrail"
            drawerAddTitle="Add Guardrail"
            drawerEditTitle="Edit Guardrail"
            drawerAddSubtitle="Choose a guardrail to configure advanced options."
            drawerEditSubtitle="Update the guardrail configuration."
            policyNoun="guardrail"
            loadingLabel="Loading guardrails..."
            searchPlaceholder="Search guardrails..."
            catalogErrorLabel="Failed to load guardrails."
            emptySearchTitle="No guardrails match your search"
            emptyCatalogTitle="No guardrails available"
            emptyCatalogDescription="No guardrail policies are available in the catalog."
            policies={guardrails}
            onAdd={handleAddGuardrail}
            onEdit={handleEditGuardrail}
            onRemove={handleRemoveGuardrail}
          />
        </Collapse>
      )}
      <Collapse in={!!formData.templateId}>
        {isSingleGatewayChoice ? (
          <EnvironmentGatewaySelector
            orgId={orgId}
            value={formData.gatewayIds ?? []}
            onChange={(ids) => handleFieldChange("gatewayIds", ids)}
            onValidityChange={setIsGatewaySelectionValid}
            onSingleChoiceChange={setIsSingleGatewayChoice}
          />
        ) : (
          <Form.Section>
            <Form.Subheader>Deployment Configuration</Form.Subheader>
            <Form.Stack spacing={2}>
              {!isLoadingGateways && Boolean(gatewaysFetchError) && (
                <Alert severity="error">
                  {getErrorMessage(gatewaysFetchError)}
                </Alert>
              )}
              {!isLoadingGateways &&
                !gatewaysFetchError &&
                gateways.length === 0 && (
                  <Alert severity="warning">
                    No egress-capable gateway is configured for this
                    organization. Ask an admin to configure one before
                    creating an LLM provider.
                  </Alert>
                )}
              <EnvironmentGatewaySelector
                orgId={orgId}
                value={formData.gatewayIds ?? []}
                onChange={(ids) => handleFieldChange("gatewayIds", ids)}
                onValidityChange={setIsGatewaySelectionValid}
                onSingleChoiceChange={setIsSingleGatewayChoice}
              />
            </Form.Stack>
          </Form.Section>
        )}
      </Collapse>
      {errorMessage && (
        <Alert severity="error">
          <Typography variant="body2">{errorMessage}</Typography>
        </Alert>
      )}

      {submittedErrorsList && (
        <Alert severity="error">
          <Typography variant="body2" component="span">
            Please fix the following:{" "}
            {submittedErrorsList.map(([, msg]) => msg).join("; ")}
          </Typography>
        </Alert>
      )}

      {/* Actions */}
      <Box
        sx={{
          mt: 2,
          display: "flex",
          gap: 1,
        }}
      >
        <Button variant="outlined" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={
            isSubmitting ||
            !formData.gatewayIds ||
            formData.gatewayIds?.length === 0 ||
            !isGatewaySelectionValid
          }
        >
          {isSubmitting ? "Creating..." : submitLabel}
        </Button>
      </Box>
    </Form.Stack>
  );
};

export default AddLLMProviderForm;
