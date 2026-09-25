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

import { useGenerateEvaluatorCode } from "@agent-management-platform/api-client";
import type { EvaluatorLevel } from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Divider,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useColorScheme,
} from "@wso2/oxygen-ui";
import {
  Check,
  Copy,
  X as CloseIcon,
} from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";
import { DiffEditor } from "@monaco-editor/react";
import { useState } from "react";
import { useParams } from "react-router-dom";

import { AI_COPILOT_PROMPT_TEMPLATE } from "../generated/evaluator-models.generated";
import { EvaluatorLlmProviderSection } from "./EvaluatorLlmProviderSection";

const MAX_INSTRUCTIONS = 4000;

const TYPE_LABELS: Record<string, string> = {
  code: "code",
  llm_judge: "LLM-judge",
};
const LEVEL_LABELS: Record<string, string> = {
  trace: "trace-level",
  agent: "agent-level",
  llm: "llm-level",
};

/**
 * Builds the copy-paste prompt for writing the evaluator in an AI assistant of the
 * user's choice. It is kept alongside the in-product generator on purpose: not every
 * org has an LLM provider configured, and some people would rather use the assistant
 * they already work in.
 */
function resolveAiPrompt(
  type: string,
  level: EvaluatorLevel,
  displayName: string,
  description: string,
): string {
  const guideUrl = `${window.location.origin}/prompts/writing-evaluators.md`;
  return AI_COPILOT_PROMPT_TEMPLATE.replace("{{TYPE}}", TYPE_LABELS[type] ?? type)
    .replace("{{LEVEL}}", LEVEL_LABELS[level] ?? level)
    .replace("{{GUIDE_URL}}", guideUrl)
    .replace("{{EVALUATOR_NAME}}", displayName || "[add name here]")
    .replace("{{EVALUATOR_DESCRIPTION}}", description || "[add description here]");
}

/**
 * Turns a failed generation into something the user can act on. The status is what
 * distinguishes the cases: a 403 is an access problem and has nothing to do with the
 * provider's credential, so it must not be reported as one.
 */
function describeGenerateError(error: unknown, artifactLabel: string): string {
  const status =
    typeof error === "object" && error !== null && "status" in error
      ? (error as { status?: number }).status
      : undefined;

  switch (status) {
    case 401:
    case 403:
      return "You do not have permission to generate evaluator source. This needs the same access as creating an evaluator (evaluator:create).";
    case 404:
      return "That LLM provider no longer exists. Pick another one.";
    case 422:
      // The backend explains which precondition failed (no upstream, no
      // credential, unsupported template), so pass its own wording through.
      return typeof error === "object" && error !== null && "message" in error
        ? String((error as { message?: unknown }).message)
        : "This LLM provider cannot be used to generate evaluator source.";
    case 502:
      return `The provider's upstream rejected the request. Check that "${artifactLabel}" is supported and that the model name is one this provider serves.`;
    default:
      return `Could not generate the ${artifactLabel}. Please try again.`;
  }
}

interface AiEvaluatorGeneratorProps {
  evaluatorType: "code" | "llm_judge";
  level: EvaluatorLevel;
  /** Passed to the model as extra context; not required. */
  displayName: string;
  description: string;
  /** Current editor content, shown as the "before" side of the diff. */
  currentSource: string;
  /** Replaces the editor content with the generated source. */
  onInsert: (code: string) => void;
  onClose: () => void;
}

/**
 * "Use AI to write" panel: pick one of the org's LLM providers, type the model to
 * use, describe the check, and get evaluator source back.
 *
 * Every field here is component-local on purpose. The panel is unmounted when it is
 * closed and when the form is left, so the provider, the model and the draft are gone
 * the next time it opens — none of it is part of the evaluator being saved, and the
 * backend stores nothing either.
 */
export function AiEvaluatorGenerator({
  evaluatorType,
  level,
  displayName,
  description,
  currentSource,
  onInsert,
  onClose,
}: AiEvaluatorGeneratorProps) {
  const { orgId } = useParams<{ orgId: string }>();

  const [providerId, setProviderId] = useState<string | undefined>(undefined);
  const [model, setModel] = useState("");
  const [instructions, setInstructions] = useState("");
  const [generated, setGenerated] = useState<string | null>(null);
  const [promptCopied, setPromptCopied] = useState(false);
  const { mode: colorSchemeMode } = useColorScheme();

  const generate = useGenerateEvaluatorCode();

  const artifactLabel = evaluatorType === "code" ? "code" : "prompt";
  const aiPrompt = resolveAiPrompt(
    evaluatorType,
    level,
    displayName,
    description,
  );
  const trimmedModel = model.trim();
  const trimmedInstructions = instructions.trim();
  const canGenerate =
    !!orgId &&
    !!providerId &&
    trimmedModel.length > 0 &&
    trimmedInstructions.length > 0 &&
    !generate.isPending;

  const handleGenerate = () => {
    if (!canGenerate) return;
    // A new run invalidates the previous suggestion — showing a stale preview next
    // to a running spinner reads as if the old result is the new one.
    setGenerated(null);
    generate.mutate(
      {
        params: { orgName: orgId, providerId },
        body: {
          model: trimmedModel,
          evaluatorType,
          level,
          instructions: trimmedInstructions,
          displayName: displayName || undefined,
          description: description || undefined,
        },
      },
      {
        onSuccess: (data) => setGenerated(data.code),
      },
    );
  };

  return (
    <Box
      sx={{
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        p: 2,
        mb: 1,
        bgcolor: "action.hover",
      }}
    >
      <Stack spacing={1.5}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="subtitle2">Use AI to write</Typography>
          <IconButton size="small" onClick={onClose} aria-label="Close">
            <CloseIcon size={16} />
          </IconButton>
        </Stack>

        <Typography variant="body2" color="text.secondary">
          Copy this prompt into any AI assistant and paste the result into the
          editor, or generate it here with one of your own LLM providers.
        </Typography>

        <TextField
          multiline
          rows={7}
          fullWidth
          value={aiPrompt}
          InputProps={{
            readOnly: true,
            sx: { fontFamily: "monospace", fontSize: "0.8rem" },
            endAdornment: (
              <InputAdornment
                position="end"
                sx={{ alignSelf: "flex-start", mt: 1, mr: -0.5 }}
              >
                <Tooltip
                  title={promptCopied ? "Copied!" : "Copy to clipboard"}
                  placement="top"
                >
                  <IconButton
                    size="small"
                    aria-label="Copy prompt"
                    onClick={() => {
                      navigator.clipboard
                        .writeText(aiPrompt)
                        .then(() => {
                          setPromptCopied(true);
                          setTimeout(() => setPromptCopied(false), 2000);
                        })
                        .catch(() => {
                          /* clipboard unavailable */
                        });
                    }}
                  >
                    {promptCopied ? <Check size={14} /> : <Copy size={14} />}
                  </IconButton>
                </Tooltip>
              </InputAdornment>
            ),
          }}
        />

        <Divider>
          <Typography variant="caption" color="text.secondary">
            or generate it here
          </Typography>
        </Divider>

        <Typography variant="body2" color="text.secondary">
          Pick one of your LLM providers and the model to use, then describe what
          the evaluator should measure. The provider and model are used for this
          generation only — nothing is saved.
        </Typography>

        <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
          <Box sx={{ flex: 1 }}>
            <EvaluatorLlmProviderSection
              selectedProviderName={providerId}
              onProviderChange={setProviderId}
            />
          </Box>
          <Box sx={{ flex: 1 }}>
            <TextInput
              size="small"
              fullWidth
              label="Model"
              placeholder="e.g. gpt-4o"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              helperText="Typed as the provider names it"
            />
          </Box>
        </Stack>

        <TextInput
          multiline
          rows={3}
          fullWidth
          size="small"
          label="What should this evaluator measure?"
          placeholder="e.g. Score whether the agent's final answer cites at least one retrieved document."
          value={instructions}
          onChange={(e) =>
            setInstructions(e.target.value.slice(0, MAX_INSTRUCTIONS))
          }
        />

        {generate.isError && (
          <Alert severity="error">
            {describeGenerateError(generate.error, artifactLabel)}
          </Alert>
        )}

        <Stack direction="row" spacing={1} alignItems="center">
          <Button
            variant="contained"
            size="small"
            onClick={handleGenerate}
            disabled={!canGenerate}
            startIcon={
              generate.isPending ? <CircularProgress size={14} /> : undefined
            }
            sx={{ textTransform: "none" }}
          >
            {generate.isPending ? "Generating…" : "Generate"}
          </Button>
          {!providerId && (
            <Typography variant="caption" color="text.secondary">
              Select an LLM provider to continue.
            </Typography>
          )}
        </Stack>

        {generated !== null && (
          <Stack spacing={1}>
            <Typography variant="subtitle2">
              Proposed changes {currentSource.trim() ? "(current \u2192 generated)" : ""}
            </Typography>
            <Box
              sx={{
                border: 1,
                borderColor: "divider",
                borderRadius: 1,
                overflow: "hidden",
                height: 320,
              }}
            >
              <DiffEditor
                height="100%"
                language={evaluatorType === "code" ? "python" : "plaintext"}
                theme={colorSchemeMode === "dark" ? "vs-dark" : "light"}
                original={currentSource}
                modified={generated}
                options={{
                  readOnly: true,
                  renderSideBySide: false,
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                  fontSize: 13,
                  lineNumbers: "on",
                  automaticLayout: true,
                }}
              />
            </Box>
            <Stack direction="row" spacing={1}>
              <Button
                variant="contained"
                size="small"
                onClick={() => onInsert(generated)}
                sx={{ textTransform: "none" }}
              >
                Insert into editor
              </Button>
              <Button
                variant="text"
                size="small"
                onClick={() => setGenerated(null)}
                sx={{ textTransform: "none" }}
              >
                Discard
              </Button>
            </Stack>
            <Typography variant="caption" color="text.secondary">
              Inserting replaces whatever is currently in the editor. Review the{" "}
              {artifactLabel} before saving.
            </Typography>
          </Stack>
        )}
      </Stack>
    </Box>
  );
}
