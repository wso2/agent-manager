/**
 * LLM Provider configuration section shown inside the EvaluatorDetailsDrawer.
 * This manages monitor-level LLM credentials (shared across llm-judge evaluators).
 */

import type {
  EvaluatorLLMProvider,
  MonitorLLMProviderConfig,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Box,
  Button,
  Chip,
  Form,
  IconButton,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { useCallback, useMemo, useState } from "react";

interface EvaluatorLlmProviderSectionProps {
  llmProviderConfigs: MonitorLLMProviderConfig[];
  onLLMProviderConfigsChange: (configs: MonitorLLMProviderConfig[]) => void;
  llmProviders: EvaluatorLLMProvider[];
}

export function EvaluatorLlmProviderSection({
  llmProviderConfigs,
  onLLMProviderConfigsChange,
  llmProviders,
}: EvaluatorLlmProviderSectionProps) {
  const [draftProviderName, setDraftProviderName] = useState("");
  const [draftEnvVariables, setDraftEnvVariables] = useState<
    Array<{ key: string; value: string }>
  >([]);

  const handleProviderChange = useCallback(
    (providerName: string) => {
      setDraftProviderName(providerName);
      const provider = llmProviders.find((p) => p.name === providerName);
      const initialRows = (provider?.configFields ?? []).map((f) => ({
        key: f.envVar,
        value: "",
      }));
      setDraftEnvVariables(initialRows);
    },
    [llmProviders],
  );

  const handleAddCredentials = useCallback(() => {
    if (!draftProviderName) return;
    const newConfigs: MonitorLLMProviderConfig[] = draftEnvVariables
      .filter((e) => e.key.trim() && e.value.trim())
      .map((e) => ({
        providerName: draftProviderName,
        envVar: e.key.trim(),
        value: e.value,
      }));
    if (newConfigs.length > 0) {
      onLLMProviderConfigsChange([...llmProviderConfigs, ...newConfigs]);
      setDraftEnvVariables([]);
      setDraftProviderName("");
    }
  }, [
    draftProviderName,
    draftEnvVariables,
    llmProviderConfigs,
    onLLMProviderConfigsChange,
  ]);

  const handleRemoveCredential = useCallback(
    (index: number) => {
      onLLMProviderConfigsChange(
        llmProviderConfigs.filter((_, i) => i !== index),
      );
    },
    [llmProviderConfigs, onLLMProviderConfigsChange],
  );

  const hasCompleteDraftRows = draftEnvVariables.some(
    (e) => e.key.trim() && e.value.trim(),
  );

  const availableProvidersToAdd = useMemo(
    () =>
      llmProviders.filter(
        (p) => !llmProviderConfigs.some((c) => c.providerName === p.name),
      ),
    [llmProviders, llmProviderConfigs],
  );

  return (
    <Box mt={1}>
      <Accordion>
        <AccordionSummary>
          <Typography variant="subtitle2">LLM Providers</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <Form.Stack flexGrow={1}>
            <Typography variant="body2" color="text.secondary">
              Configured LLM Providers
            </Typography>
            <Form.ElementWrapper label="Provider" name="draftProvider">
              <TextField
                select
                size="small"
                fullWidth
                value={draftProviderName}
                onChange={(e) => handleProviderChange(e.target.value)}
              >
                <MenuItem value="">Select a provider</MenuItem>
                {availableProvidersToAdd.map((p) => (
                  <MenuItem key={p.name} value={p.name}>
                    {p.displayName}
                  </MenuItem>
                ))}
              </TextField>
            </Form.ElementWrapper>

            {availableProvidersToAdd.length === 0 &&
              llmProviderConfigs.length > 0 && (
                <Typography variant="body2" color="text.secondary">
                  All providers have been added. Remove one above to add it
                  again.
                </Typography>
              )}

            {draftProviderName && (
              <>
                {draftEnvVariables.length > 0 ? (
                  <Form.Stack flexGrow={1}>
                    {draftEnvVariables.map((envVar, index) => (
                      <Form.ElementWrapper
                        key={envVar.key}
                        label={envVar.key}
                        name={`llm-env-${envVar.key}`}
                      >
                        <TextField
                          size="small"
                          fullWidth
                          type="password"
                          placeholder="API key or secret"
                          value={envVar.value}
                          onChange={(e) => {
                            const next = [...draftEnvVariables];
                            next[index] = {
                              ...envVar,
                              value: e.target.value,
                            };
                            setDraftEnvVariables(next);
                          }}
                        />
                      </Form.ElementWrapper>
                    ))}
                  </Form.Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    No configs required for this LLM provider.
                  </Typography>
                )}
                <Box>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={<Plus size={16} />}
                    disabled={!hasCompleteDraftRows}
                    onClick={handleAddCredentials}
                  >
                    Add credentials
                  </Button>
                </Box>
              </>
            )}
            {llmProviderConfigs.length > 0 && (
              <Stack spacing={1}>
                {llmProviderConfigs.map((cred, index) => {
                  const provider = llmProviders.find(
                    (p) => p.name === cred.providerName,
                  );
                  return (
                    <Stack
                      key={`${cred.providerName}-${cred.envVar}-${index}`}
                      direction="row"
                      spacing={2}
                      alignItems="center"
                      sx={{
                        p: 1.5,
                        border: 1,
                        borderColor: "divider",
                        borderRadius: 1,
                      }}
                    >
                      <Stack
                        direction="row"
                        spacing={1}
                        flexGrow={1}
                        alignItems="center"
                      >
                        <Chip
                          size="small"
                          label={provider?.displayName ?? cred.providerName}
                          color="primary"
                          variant="outlined"
                        />
                        <Chip
                          size="small"
                          label={cred.envVar + " : ****..."}
                          variant="outlined"
                        />
                      </Stack>
                      <Tooltip title="Remove">
                        <IconButton
                          size="small"
                          color="error"
                          onClick={() => handleRemoveCredential(index)}
                        >
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  );
                })}
              </Stack>
            )}
          </Form.Stack>
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}
