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

import {
  type KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import type {
  LLMModel,
  LLMModelProvider,
  LLMProviderResponse,
  UpdateLLMProviderRequest,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Chip,
  Collapse,
  Drawer,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Inbox, Package, Plus, ServerCog, X } from "@wso2/oxygen-ui-icons-react";
import { z } from "zod";

const LLMModelSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  description: z.string().optional(),
});

const ModelProviderSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  models: z.array(LLMModelSchema),
});

const ModelProvidersPayloadSchema = z.object({
  modelProviders: z.array(ModelProviderSchema),
});

const newModelNameSchema = z.string().min(1).trim();

const PROVIDER_OPTIONS: { id: string; name: string; description?: string }[] = [
  { id: "openai", name: "OpenAI", description: "GPT models" },
  { id: "anthropic", name: "Anthropic", description: "Claude models" },
  { id: "gemini", name: "Google AI Studio", description: "Gemini models" },
  { id: "groq", name: "Groq", description: "Fast inference" },
  { id: "mistral", name: "Mistral AI", description: "Mistral models" },
];

const MAX_MODEL_PROVIDERS = 1;

export type LLMProviderModelsTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  isLoading?: boolean;
  error?: Error | null;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderModelsTab({
  providerData,
  isLoading = false,
  error: providerError = null,
  onUpdate,
  isUpdating,
}: LLMProviderModelsTabProps) {

  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error" | "warning";
  } | null>(null);
  const [selectedProviderIndex, setSelectedProviderIndex] = useState<
    number | null
  >(null);
  const [addModelInput, setAddModelInput] = useState("");
  const [addModelError, setAddModelError] = useState<string | null>(null);

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerSelectedOption, setDrawerSelectedOption] = useState<string | null>(
    null,
  );

  const modelProviders = useMemo(
    () => providerData?.modelProviders ?? [],
    [providerData?.modelProviders],
  );
  const selectedProvider =
    selectedProviderIndex !== null &&
    selectedProviderIndex >= 0 &&
    selectedProviderIndex < modelProviders.length
      ? modelProviders[selectedProviderIndex]
      : null;

  useEffect(() => {
    if (modelProviders.length > 0 && selectedProviderIndex === null) {
      setSelectedProviderIndex(0);
    }
    if (modelProviders.length === 0) {
      setSelectedProviderIndex(null);
    }
    if (
      selectedProviderIndex !== null &&
      selectedProviderIndex >= modelProviders.length
    ) {
      setSelectedProviderIndex(
        modelProviders.length > 0 ? modelProviders.length - 1 : null,
      );
    }
  }, [modelProviders.length, selectedProviderIndex]);

  const validateAndUpdate = useCallback(
    async (nextModelProviders: LLMModelProvider[]) => {
      const result = ModelProvidersPayloadSchema.safeParse({
        modelProviders: nextModelProviders,
      });
      if (!result.success) {
        const first = result.error.issues[0];
        setStatus({
          message: first?.message ?? "Validation failed",
          severity: "error",
        });
        return;
      }

      try {
        await onUpdate({
          modelProviders: result.data.modelProviders,
        });
        setStatus({
          message: "Model provider added successfully.",
          severity: "success",
        });
        setDrawerOpen(false);
        setDrawerSelectedOption(null);
      } catch {
        setStatus({
          message: "Failed to add model provider.",
          severity: "error",
        });
      }
    },
    [onUpdate],
  );

  const handleAddProvider = useCallback(() => {
    if (!drawerSelectedOption || modelProviders.length >= MAX_MODEL_PROVIDERS) {
      if (modelProviders.length >= MAX_MODEL_PROVIDERS) {
        setStatus({
          message:
            "Only one model provider is supported for this service provider.",
          severity: "warning",
        });
      }
      return;
    }

    const option = PROVIDER_OPTIONS.find((o) => o.id === drawerSelectedOption);
    if (!option) return;

    const newProvider: LLMModelProvider = {
      id: option.id,
      name: option.name,
      models: [],
    };

    validateAndUpdate([...modelProviders, newProvider]);
  }, [
    drawerSelectedOption,
    modelProviders,
    validateAndUpdate,
  ]);

  const handleRemoveProvider = useCallback(
    async (index: number) => {
      const next = modelProviders.filter((_, i) => i !== index);
      const result = ModelProvidersPayloadSchema.safeParse({
        modelProviders: next,
      });
      if (!result.success) {
        setStatus({
          message: result.error.issues[0]?.message ?? "Validation failed",
          severity: "error",
        });
        return;
      }

      try {
        await onUpdate({
          modelProviders: result.data.modelProviders,
        });
        setSelectedProviderIndex(
          index === 0 && next.length > 0 ? 0 : Math.max(0, index - 1),
        );
        setStatus({
          message: "Model provider removed successfully.",
          severity: "success",
        });
      } catch {
        setStatus({
          message: "Failed to remove model provider.",
          severity: "error",
        });
      }
    },
    [modelProviders, onUpdate],
  );

  const handleAddModel = useCallback(
    async (modelId: string) => {
      if (!selectedProvider) return;

      const trimmed = modelId.trim();
      const parsed = newModelNameSchema.safeParse(trimmed);
      if (!parsed.success) {
        setAddModelError("Model id is required");
        return;
      }

      const models = selectedProvider.models ?? [];
      const exists = models.some(
        (m) =>
          m.id?.toLowerCase() === trimmed.toLowerCase() ||
          (m.name ?? m.id)?.toLowerCase() === trimmed.toLowerCase(),
      );
      if (exists) {
        setStatus({
          message: "This model already exists for the selected provider.",
          severity: "warning",
        });
        return;
      }

      const newModel: LLMModel = {
        id: trimmed,
        name: trimmed,
      };

      const nextModels = [...models, newModel];
      const providerIndex = selectedProviderIndex!;
      const nextProviders = modelProviders.map((mp, i) =>
        i === providerIndex ? { ...mp, models: nextModels } : mp,
      );

      const result = ModelProvidersPayloadSchema.safeParse({
        modelProviders: nextProviders,
      });
      if (!result.success) {
        setStatus({
          message: result.error.issues[0]?.message ?? "Validation failed",
          severity: "error",
        });
        return;
      }

      try {
        await onUpdate({
          modelProviders: result.data.modelProviders,
        });
        setAddModelInput("");
        setAddModelError(null);
        setStatus({
          message: "Model added successfully.",
          severity: "success",
        });
      } catch {
        setStatus({
          message: "Failed to add model.",
          severity: "error",
        });
      }
    },
    [
      selectedProvider,
      selectedProviderIndex,
      modelProviders,
      onUpdate,
    ],
  );

  const handleRemoveModel = useCallback(
    async (modelId: string) => {
      if (!selectedProvider) return;

      const models = (selectedProvider.models ?? []).filter(
        (m) => m.id !== modelId,
      );
      const providerIndex = selectedProviderIndex!;
      const nextProviders = modelProviders.map((mp, i) =>
        i === providerIndex ? { ...mp, models } : mp,
      );

      const result = ModelProvidersPayloadSchema.safeParse({
        modelProviders: nextProviders,
      });
      if (!result.success) {
        setStatus({
          message: result.error.issues[0]?.message ?? "Validation failed",
          severity: "error",
        });
        return;
      }

      try {
        await onUpdate({
          modelProviders: result.data.modelProviders,
        });
        setStatus({
          message: "Model removed successfully.",
          severity: "success",
        });
      } catch {
        setStatus({
          message: "Failed to remove model.",
          severity: "error",
        });
      }
    },
    [
      selectedProvider,
      selectedProviderIndex,
      modelProviders,
      onUpdate,
    ],
  );

  const handleAddModelKeyDown = (
    e: KeyboardEvent<HTMLInputElement>,
  ) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void handleAddModel(addModelInput);
    }
  };

  const isSaving = isUpdating;
  const atMaxProviders = modelProviders.length >= MAX_MODEL_PROVIDERS;
  const addedProviderIds = new Set(modelProviders.map((mp) => mp.id));

  if (isLoading) {
    return (
      <Stack spacing={3}>
        <Skeleton variant="rounded" height={120} />
        <Skeleton variant="rounded" height={200} />
      </Stack>
    );
  }

  if (!providerData && !providerError) {
    return null;
  }

  return (
    <Stack spacing={3}>
      {providerError && (
        <Alert severity="error" sx={{ width: "100%" }}>
          {providerError instanceof Error
            ? providerError.message
            : "Failed to load provider."}
        </Alert>
      )}

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

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 4 }} sx={{ minWidth: 280 }}>
          <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
            Model Provider
          </Typography>

          {modelProviders.length === 0 ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<ServerCog size={64} />}
                title="No providers added yet"
                description="Add a model provider to manage its models."
                action={
                  <Button
                    variant="contained"
                    startIcon={<Plus size={16} />}
                    onClick={() => setDrawerOpen(true)}
                    disabled={isSaving}
                  >
                    Add Model Provider
                  </Button>
                }
              />
            </ListingTable.Container>
          ) : (
            <Stack spacing={1}>
              {modelProviders.map((mp, index) => (
                <Box
                  key={mp.id}
                  onClick={() => setSelectedProviderIndex(index)}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    p: 1.5,
                    border: "1px solid",
                    borderColor:
                      selectedProviderIndex === index
                        ? "primary.main"
                        : "divider",
                    borderRadius: 1,
                    bgcolor:
                      selectedProviderIndex === index
                        ? "action.selected"
                        : "background.paper",
                    cursor: "pointer",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                >
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {mp.name ?? mp.id}
                  </Typography>
                  <IconButton
                    size="small"
                    color="error"
                    onClick={(e) => {
                      e.stopPropagation();
                      void handleRemoveProvider(index);
                    }}
                    disabled={isSaving}
                    aria-label="Remove provider"
                  >
                    <X size={16} />
                  </IconButton>
                </Box>
              ))}
              <Tooltip
                title={
                  atMaxProviders
                    ? "Only one model provider is supported for this service provider."
                    : ""
                }
              >
                <span>
                  <Button
                    variant="outlined"
                    startIcon={<Plus size={16} />}
                    onClick={() => setDrawerOpen(true)}
                    disabled={atMaxProviders || isSaving}
                    fullWidth
                  >
                    Add Model Provider
                  </Button>
                </span>
              </Tooltip>
            </Stack>
          )}
        </Grid>

        <Grid size={{ xs: 12, md: 8 }} sx={{ flex: 1 }}>
          <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
            Models Available
            {selectedProvider && ` — ${selectedProvider.name ?? selectedProvider.id}`}
          </Typography>

          {!selectedProvider ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<Inbox size={64} />}
                title="Select a provider to see models"
                description="Choose a model provider from the list on the left."
              />
            </ListingTable.Container>
          ) : (
            <Stack spacing={2}>
              <FormControl fullWidth size="small">
                <FormLabel>Type model id and press Enter</FormLabel>
                <TextField
                  size="small"
                  placeholder="e.g., gpt-4.1-mini"
                  value={addModelInput}
                  onChange={(e) => {
                    setAddModelInput(e.target.value);
                    setAddModelError(null);
                  }}
                  onKeyDown={handleAddModelKeyDown}
                  error={!!addModelError}
                  helperText={addModelError}
                  disabled={isSaving}
                />
              </FormControl>

              <Stack
                direction="row"
                spacing={1}
                sx={{ flexWrap: "wrap", gap: 1 }}
              >
                {(selectedProvider.models ?? []).length === 0 ? (
                  <ListingTable.Container sx={{ width: "100%" }}>
                    <ListingTable.EmptyState
                      illustration={<Package size={64} />}
                      title="No models found for this provider yet"
                      description="Type a model id above and press Enter to add."
                    />
                  </ListingTable.Container>
                ) : (
                  (selectedProvider.models ?? []).map((model) => (
                    <Chip
                      key={model.id}
                      label={model.name ?? model.id}
                      onDelete={() => void handleRemoveModel(model.id)}
                      disabled={isSaving}
                      size="small"
                      variant="outlined"
                    />
                  ))
                )}
              </Stack>
            </Stack>
          )}
        </Grid>
      </Grid>

      <Drawer
        anchor="right"
        open={drawerOpen}
        onClose={() => {
          setDrawerOpen(false);
          setDrawerSelectedOption(null);
        }}
        slotProps={{ paper: { sx: { width: 420 } } }}
      >
        <Box sx={{ p: 3, display: "flex", flexDirection: "column", height: "100%" }}>
          <Typography variant="h6" component="h2" sx={{ mb: 0.5 }}>
            Add Model Provider
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Select a provider to add its model catalog.
          </Typography>

          <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
            {PROVIDER_OPTIONS.map((opt) => {
              const isAdded = addedProviderIds.has(opt.id);
              const isSelected = drawerSelectedOption === opt.id;
              return (
                <Box
                  key={opt.id}
                  onClick={() =>
                    !isAdded && setDrawerSelectedOption(opt.id)
                  }
                  sx={{
                    p: 2,
                    border: "1px solid",
                    borderColor: isSelected ? "primary.main" : "divider",
                    borderRadius: 1,
                    bgcolor: isSelected ? "action.selected" : "background.paper",
                    cursor: isAdded ? "default" : "pointer",
                    opacity: isAdded ? 0.7 : 1,
                    "&:hover": isAdded ? {} : { bgcolor: "action.hover" },
                  }}
                >
                  <Stack
                    direction="row"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Stack>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {opt.name}
                      </Typography>
                      {opt.description && (
                        <Typography variant="caption" color="text.secondary">
                          {opt.description}
                        </Typography>
                      )}
                    </Stack>
                    {isAdded && (
                      <Chip label="Added" size="small" color="success" />
                    )}
                  </Stack>
                </Box>
              );
            })}
          </Stack>

          <Stack direction="row" spacing={1.5} justifyContent="flex-end" sx={{ mt: 2, pt: 2, borderTop: 1, borderColor: "divider" }}>
            <Button
              onClick={() => {
                setDrawerOpen(false);
                setDrawerSelectedOption(null);
              }}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleAddProvider}
              disabled={
                !drawerSelectedOption ||
                isSaving ||
                addedProviderIds.has(drawerSelectedOption)
              }
            >
              Add
            </Button>
          </Stack>
        </Box>
      </Drawer>
    </Stack>
  );
}
