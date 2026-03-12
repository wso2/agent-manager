/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  CardContent,
  Collapse,
  Form,
  FormControl,
  FormLabel,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Brain } from "@wso2/oxygen-ui-icons-react";
import {
  addLLMProviderSchema,
  type AddLLMProviderFormValues,
} from "../form/schema";
import { useValidatedForm } from "../hooks/useValidatedForm";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "./GuardrailsSection";
import { useListGateways } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";

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
   * Auth type from template metadata (e.g., "bearer", "apiKey").
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
  templates: TemplateCard[];
  isLoadingTemplates: boolean;
  missingParamsMessage?: string | null;
  errorMessage?: string | null;
  isSubmitting?: boolean;
  onCancel: () => void;
  onSubmit: (
    values: AddLLMProviderFormValues,
    guardrails: GuardrailSelection[],
  ) => void;
}

const INITIAL_FORM_VALUES: AddLLMProviderFormValues = {
  templateId: "",
  displayName: "",
  version: "v1.0",
  description: "",
  context: "",
  upstreamUrl: "",
  apiKey: "",
  gatewayIds: [],
};

export const AddLLMProviderForm: React.FC<AddLLMProviderFormProps> = ({
  templates,
  isLoadingTemplates,
  missingParamsMessage,
  errorMessage,
  isSubmitting,
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

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === formData.templateId) ?? null,
    [formData.templateId, templates],
  );

  const { orgId } = useParams<{ orgId: string }>();
  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
  );

  const gateways = useMemo(
    () => gatewaysData?.gateways ?? [],
    [gatewaysData?.gateways],
  );

  const hasTemplateUrl = Boolean(selectedTemplate?.hasTemplateUrl);
  const requiresUpstream = !hasTemplateUrl;
  const requiresApiKey = !selectedTemplate?.hasTemplateAuthHeader;

  const [guardrails, setGuardrails] = useState<GuardrailSelection[]>([]);

  const handleAddGuardrail = useCallback((guardrail: GuardrailSelection) => {
    setGuardrails((prev) => {
      if (prev.some((g) => g.name === guardrail.name)) return prev;
      return [...prev, guardrail];
    });
  }, []);

  const handleRemoveGuardrail = useCallback((name: string) => {
    setGuardrails((prev) => prev.filter((g) => g.name !== name));
  }, []);

  const showLoading = isLoadingTemplates || isLoadingGateways;

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

  const handleTemplateSelect = useCallback(
    (templateId: string) => {
      setFormData((prev) => {
        const next: AddLLMProviderFormValues = {
          ...prev,
          templateId,
          upstreamUrl: "",
        };
        const fieldError = validateField("templateId", templateId, next);
        setFieldError("templateId", fieldError);
        // Clear any stale upstream error when switching templates.
        setFieldError("upstreamUrl", undefined);
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
        apiKey: formData.apiKey?.trim() ?? "",
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
    <Stack spacing={3}>
      {missingParamsMessage && (
        <Typography color="error" variant="body2">
          {missingParamsMessage}
        </Typography>
      )}



      {/* Template selector */}

      <Form.Section>
        <Form.Header>Basic Details</Form.Header>
        <Form.Stack spacing={2}>
          <Form.Stack
            direction={{ xs: "column", md: "row" }}
            spacing={2}
            useFlexGap
          >
            <FormControl sx={{ flex: 2 }} error={Boolean(errors.displayName)}>
              <FormLabel required>Name</FormLabel>
              <TextField
                fullWidth
                value={formData.displayName}
                onChange={(e) =>
                  handleFieldChange("displayName", e.target.value)
                }
                placeholder="Production OpenAI Provider"
                error={Boolean(errors.displayName)}
                helperText={errors.displayName}
              />
            </FormControl>

            <FormControl sx={{ flex: 1 }} error={Boolean(errors.version)}>
              <FormLabel required>Version</FormLabel>
              <TextField
                fullWidth
                value={formData.version}
                onChange={(e) => handleFieldChange("version", e.target.value)}
                placeholder="v1.0"
                error={Boolean(errors.version)}
                helperText={errors.version}
              />
            </FormControl>
          </Form.Stack>

          <FormControl fullWidth error={Boolean(errors.description)}>
            <FormLabel>Short description</FormLabel>
            <TextField
              fullWidth
              multiline
              rows={2}
              value={formData.description ?? ""}
              onChange={(e) => handleFieldChange("description", e.target.value)}
              placeholder="Primary LLM provider for production"
              error={Boolean(errors.description)}
              helperText={errors.description}
            />
          </FormControl>

          <FormControl fullWidth error={Boolean(errors.gatewayIds)}>
            <FormLabel>Gateway</FormLabel>
            {isLoadingGateways ? (
              <Skeleton variant="rounded" height={40} sx={{ mt: 0.5 }} />
            ) : (
              <Autocomplete
                multiple
                options={gateways}
                size="small"
                value={gateways.filter((g) =>
                  (formData.gatewayIds ?? []).includes(g.uuid),
                )}
                onChange={(_, newValue) => {
                  handleFieldChange(
                    "gatewayIds",
                    newValue.map((g) => g.uuid),
                  );
                }}
                getOptionLabel={(option) =>
                  option.displayName || option.name || option.uuid
                }
                renderInput={(params) => (
                  <TextField
                    {...params}
                    placeholder="Select gateway(s)"
                    error={Boolean(errors.gatewayIds)}
                  />
                )}
                sx={{ mt: 0.5 }}
              />
            )}
            {errors.gatewayIds && (
              <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
                {errors.gatewayIds}
              </Typography>
            )}
          </FormControl>
        </Form.Stack>
      </Form.Section>

      {showLoading && (
        <Box>
          <Skeleton variant="text" width={140} height={20} sx={{ mb: 1.5 }} />
          <Box
            sx={{
              display: "grid",
              gap: 1.5,
              gridTemplateColumns: {
                xs: "1fr",
                sm: "repeat(3, 1fr)",
                md: "repeat(4, 1fr)",
                lg: "repeat(4, 1fr)",
                xl: "repeat(6, 1fr)",
              },
            }}
          >
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} variant="rounded" height={72} />
            ))}
          </Box>
        </Box>
      )}

      <Form.Section>
        <Form.Header>Provider Template</Form.Header>
        <FormControl fullWidth>
          {isLoadingTemplates ? (
            <Skeleton
              variant="rounded"
              height={120}
              sx={{ mt: 1.5, maxWidth: 600 }}
            />
          ) : (
            <Box
              sx={{
                mt: 1.5,
                display: "grid",
                gap: 1.5,
                gridTemplateColumns: {
                  xs: "1fr",
                  sm: "repeat(3, 1fr)",
                  md: "repeat(4, 1fr)",
                  lg: "repeat(4, 1fr)",
                  xl: "repeat(6, 1fr)",
                },
              }}
            >
              {templates.map((template) => {
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
                              backgroundColor: "grey.100",
                              borderRadius: "20%",
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
              {!templates.length && !isLoadingTemplates && (
                <Typography variant="body2" color="text.secondary">
                  No provider templates available for this organization.
                </Typography>
              )}
            </Box>
          )}
        </FormControl>
        <Collapse in={!!formData.templateId}>
          <Stack spacing={2}>
            <FormControl fullWidth error={Boolean(errors.context)}>
              <FormLabel>Context path</FormLabel>
              <TextField
                fullWidth
                value={formData.context ?? ""}
                onChange={(e) => handleFieldChange("context", e.target.value)}
                placeholder="/my-provider"
                error={Boolean(errors.context)}
                helperText={
                  errors.context ??
                  "API context path (must start with /, no trailing slash)"
                }
              />
            </FormControl>
            <Collapse in={!hasTemplateUrl}>
              <FormControl fullWidth error={Boolean(errors.upstreamUrl)}>
                <FormLabel required>Upstream endpoint</FormLabel>
                <TextField
                  fullWidth
                  value={formData.upstreamUrl ?? ""}
                  onChange={(e) => handleFieldChange("upstreamUrl", e.target.value)}
                  placeholder="https://api.openai.com/v1"
                  error={Boolean(errors.upstreamUrl)}
                  helperText={errors.upstreamUrl}
                />
              </FormControl>
            </Collapse>

            <FormControl fullWidth error={Boolean(errors.apiKey)}>
              <FormLabel required={requiresApiKey}>API key / credential</FormLabel>
              <TextField
                fullWidth
                type="password"
                value={formData.apiKey}
                onChange={(e) => handleFieldChange("apiKey", e.target.value)}
                placeholder="Enter your API key"
                error={Boolean(errors.apiKey)}
                helperText={errors.apiKey}
              />
            </FormControl>
          </Stack>
        </Collapse>
      </Form.Section>
      {/* Guardrails */}
      <Collapse in={!!formData.templateId}>
        <GuardrailsSection
          guardrails={guardrails}
          onAddGuardrail={handleAddGuardrail}
          onRemoveGuardrail={handleRemoveGuardrail}
        />
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
          disabled={isSubmitting}
        >
          {isSubmitting ? "Creating..." : "Add provider"}
        </Button>
      </Box>
    </Stack>
  );
};

export default AddLLMProviderForm;
