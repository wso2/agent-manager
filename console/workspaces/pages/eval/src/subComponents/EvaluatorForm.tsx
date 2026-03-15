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

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  Collapse,
  Divider,
  Form,
  FormControlLabel,
  IconButton,
  InputAdornment,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useColorScheme,
} from "@wso2/oxygen-ui";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle,
  Circle,
  Copy,
  Sparkles as SparklesIcon,
  X as CloseIcon,
} from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";
import Editor, { type Monaco } from "@monaco-editor/react";
import type { EvaluatorConfigParam, EvaluatorLevel } from "@agent-management-platform/types";
import {
  DataModelReferenceDrawer,
  type ReferenceTypeKey,
} from "./DataModelReferenceDrawer";
import {
  AI_COPILOT_PROMPTS,
  CODE_TEMPLATES,
  LLM_JUDGE_BASE_CONFIG_SCHEMA,
  LLM_JUDGE_TEMPLATES,
  LLM_JUDGE_VARIABLES,
  COMPLETIONS,
  COMMON_COMPLETIONS,
  HOVER_DOCS,
  SUPPORTED_PACKAGES,
  type CompletionSuggestion,
} from "../generated/evaluator-models.generated";

// ---------------------------------------------------------------------------
// AI copilot prompt helper
// ---------------------------------------------------------------------------

function resolveAiPrompt(type: string, level: EvaluatorLevel): string {
  const raw = AI_COPILOT_PROMPTS[type as "code" | "llm_judge"]?.[level] ?? "";
  const guideUrl = `${window.location.origin}/prompts/writing-evaluators.md`;
  return raw.replace("{{GUIDE_URL}}", guideUrl);
}

// ---------------------------------------------------------------------------
// Monaco editor providers — completions + hover docs
// ---------------------------------------------------------------------------

// Custom language for LLM judge prompts — plain text with {expression} placeholders
const LLM_JUDGE_LANG = "llm-judge-prompt";

// Module-level state so globally-registered Monaco providers can read the
// current evaluation level without fragile text scanning.
let _currentLevel: EvaluatorLevel = "trace";

function registerEditorProviders(monaco: Monaco) {
  type ProviderArgs = Parameters<
    typeof monaco.languages.registerCompletionItemProvider
  >;
  const completionProvider: ProviderArgs[1] = {
    triggerCharacters: ["."],
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    provideCompletionItems: (model: any, position: any) => {
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      // For Python (code evaluator), detect level from type hints in source.
      // For LLM judge, fall back to _currentLevel set by the form component.
      const fullText = model.getValue();
      let levelSuggestions: CompletionSuggestion[] = COMPLETIONS[_currentLevel];
      if (fullText.includes("AgentTrace") || fullText.includes("agent_trace")) {
        levelSuggestions = COMPLETIONS.agent;
      } else if (
        fullText.includes("LLMSpan") ||
        fullText.includes("llm_span")
      ) {
        levelSuggestions = COMPLETIONS.llm;
      }

      const kindMap: Record<CompletionSuggestion["kind"], number> = {
        Class: monaco.languages.CompletionItemKind.Class,
        Function: monaco.languages.CompletionItemKind.Function,
        Method: monaco.languages.CompletionItemKind.Method,
        Property: monaco.languages.CompletionItemKind.Property,
        Snippet: monaco.languages.CompletionItemKind.Snippet,
      };

      const allItems = [...COMMON_COMPLETIONS, ...levelSuggestions];

      return {
        suggestions: allItems.map((item) => ({
          label: item.label,
          kind: kindMap[item.kind],
          insertText: item.insertText,
          insertTextRules: item.snippet
            ? monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet
            : undefined,
          detail: item.detail,
          documentation: item.documentation,
          sortText: item.sortText,
          range,
        })),
      };
    },
  };

  const hoverProvider: Parameters<
    typeof monaco.languages.registerHoverProvider
  >[1] = {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    provideHover: (model: any, position: any) => {
      const word = model.getWordAtPosition(position);
      if (!word) return null;

      const hoverInfo = HOVER_DOCS[word.word];
      if (!hoverInfo) return null;

      return {
        range: {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        },
        contents: [
          { value: `\`\`\`python\n${hoverInfo.type}\n\`\`\`` },
          { value: hoverInfo.doc },
        ],
      };
    },
  };

  // Register for both Python (code evaluator) and LLM judge prompt language
  monaco.languages.registerCompletionItemProvider("python", completionProvider);
  monaco.languages.registerHoverProvider("python", hoverProvider);
  monaco.languages.registerCompletionItemProvider(
    LLM_JUDGE_LANG,
    completionProvider,
  );
  monaco.languages.registerHoverProvider(LLM_JUDGE_LANG, hoverProvider);
}

// ---------------------------------------------------------------------------
// Monaco themes
// ---------------------------------------------------------------------------

const EVAL_DARK_THEME = "eval-dark";
const EVAL_LIGHT_THEME = "eval-light";

function registerLLMJudgeLanguage(monaco: Monaco) {
  monaco.languages.register({ id: LLM_JUDGE_LANG });

  monaco.languages.setMonarchTokensProvider(LLM_JUDGE_LANG, {
    tokenizer: {
      root: [
        // Escaped braces {{ and }} — render as plain text
        [/\{\{/, "string"],
        [/\}\}/, "string"],
        // Opening brace — enter f-string expression
        [/\{/, { token: "delimiter.bracket", next: "@fstring" }],
        // Everything else is plain prompt text
        [/./, "string"],
      ],
      fstring: [
        // Nested braces (e.g., dict literals inside expressions)
        [/\{/, { token: "delimiter.bracket", next: "@fstring" }],
        // Closing brace — back to parent state
        [/\}/, { token: "delimiter.bracket", next: "@pop" }],
        // Python string literals inside expressions
        [/"[^"]*"/, "string.python"],
        [/'[^']*'/, "string.python"],
        // Numbers
        [/\b\d+(\.\d+)?\b/, "number"],
        // Method calls / function calls
        [/[a-zA-Z_]\w*(?=\()/, "identifier.method"],
        // Dotted attribute access
        [/\./, "delimiter"],
        // Identifiers (variable names, attributes)
        [/[a-zA-Z_]\w*/, "identifier"],
        // Operators and punctuation
        [/[,()[\]:+\-*/%=<>!&|~^]/, "delimiter"],
        // Whitespace
        [/\s+/, "white"],
      ],
    },
  });
}

function defineEditorThemes(monaco: Monaco) {
  const fstringRulesDark = [
    { token: "string.llm-judge-prompt", foreground: "CE9178" },
    {
      token: "delimiter.bracket.llm-judge-prompt",
      foreground: "FFD700",
      fontStyle: "bold",
    },
    { token: "identifier.llm-judge-prompt", foreground: "9CDCFE" },
    { token: "identifier.method.llm-judge-prompt", foreground: "DCDCAA" },
    { token: "number.llm-judge-prompt", foreground: "B5CEA8" },
    { token: "string.python.llm-judge-prompt", foreground: "CE9178" },
    { token: "delimiter.llm-judge-prompt", foreground: "D4D4D4" },
  ];

  const fstringRulesLight = [
    { token: "string.llm-judge-prompt", foreground: "A31515" },
    {
      token: "delimiter.bracket.llm-judge-prompt",
      foreground: "B8860B",
      fontStyle: "bold",
    },
    { token: "identifier.llm-judge-prompt", foreground: "001080" },
    { token: "identifier.method.llm-judge-prompt", foreground: "795E26" },
    { token: "number.llm-judge-prompt", foreground: "098658" },
    { token: "string.python.llm-judge-prompt", foreground: "A31515" },
    { token: "delimiter.llm-judge-prompt", foreground: "383838" },
  ];

  monaco.editor.defineTheme(EVAL_DARK_THEME, {
    base: "vs-dark",
    inherit: true,
    rules: [...fstringRulesDark],
    colors: {},
  });
  monaco.editor.defineTheme(EVAL_LIGHT_THEME, {
    base: "vs",
    inherit: true,
    rules: [...fstringRulesLight],
    colors: {},
  });
}

// ---------------------------------------------------------------------------
// Field validation — derive valid fields from generated COMPLETIONS data
// ---------------------------------------------------------------------------

function buildValidFields(): Record<EvaluatorLevel, Set<string>> {
  const result: Record<string, Set<string>> = {};
  for (const level of ["trace", "agent", "llm"] as const) {
    const fields = new Set<string>();
    for (const item of COMPLETIONS[level]) {
      if (item.label.includes(".")) {
        // Strip () for methods — "trace.get_agents()" → "trace.get_agents"
        fields.add(item.label.replace(/\(\)$/, ""));
      }
    }
    result[level] = fields;
  }
  return result as Record<EvaluatorLevel, Set<string>>;
}

const VALID_FIELDS = buildValidFields();

function validateFieldReferences(
  monaco: Monaco,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  model: any,
  level: EvaluatorLevel,
  isLLMJudge: boolean,
) {
  const rootVar = LLM_JUDGE_VARIABLES[level].varName;
  const validFields = VALID_FIELDS[level];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const markers: any[] = [];
  const text = model.getValue();

  // Use a negative lookbehind to skip dotted paths like "trace.models" in
  // "from amp_evaluation.trace.models import Trace" — only match when the
  // root variable is preceded by a word boundary (not another dot).
  const pattern = new RegExp(`(?<![.\\w])${rootVar}\\.([a-zA-Z_]\\w*)`, "g");

  let match;
  while ((match = pattern.exec(text)) !== null) {
    const fullRef = `${rootVar}.${match[1]}`;
    if (validFields.has(fullRef)) continue;

    // For LLM judge, only flag references inside {} expressions
    if (isLLMJudge) {
      const before = text.slice(0, match.index);
      const openBraces = (before.match(/{/g) || []).length;
      const closeBraces = (before.match(/}/g) || []).length;
      if (openBraces <= closeBraces) continue;
    }

    // Skip references inside string literals and comments
    const before = text.slice(0, match.index);
    const lastNewline = before.lastIndexOf("\n");
    const lineBeforeMatch = before.slice(lastNewline + 1);
    if (lineBeforeMatch.trimStart().startsWith("#")) continue;
    if (
      /(['"]).*$/.test(lineBeforeMatch) &&
      !lineBeforeMatch.endsWith(match[0])
    )
      continue;

    const startPos = model.getPositionAt(match.index + rootVar.length + 1);
    const endPos = model.getPositionAt(match.index + match[0].length);

    markers.push({
      severity: monaco.MarkerSeverity.Warning,
      message: `Unknown field '${match[1]}' on ${rootVar}`,
      startLineNumber: startPos.lineNumber,
      startColumn: startPos.column,
      endLineNumber: endPos.lineNumber,
      endColumn: endPos.column,
    });
  }

  monaco.editor.setModelMarkers(model, "evaluator-validation", markers);
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getCodeTemplate(level: EvaluatorLevel): string {
  return CODE_TEMPLATES[level];
}

function getLLMJudgeTemplate(level: EvaluatorLevel): string {
  return LLM_JUDGE_TEMPLATES[level];
}

function getTemplate(
  type: "code" | "llm_judge",
  level: EvaluatorLevel,
): string {
  return type === "code" ? getCodeTemplate(level) : getLLMJudgeTemplate(level);
}

// ---------------------------------------------------------------------------
// Form
// ---------------------------------------------------------------------------

export interface EvaluatorFormValues {
  displayName: string;
  description: string;
  type: "code" | "llm_judge";
  level: "trace" | "agent" | "llm";
  source: string;
  configSchema: EvaluatorConfigParam[];
  tags: string[];
}

const defaultValues: EvaluatorFormValues = {
  displayName: "",
  description: "",
  type: "code",
  level: "trace",
  source: CODE_TEMPLATES.trace,
  configSchema: [],
  tags: [],
};

const PARAM_TYPES = ["string", "integer", "float", "boolean", "array", "enum"] as const;

// Reusable card selector matching the InputInterface pattern from agent creation.
interface OptionCardProps {
  label: string;
  description: string;
  selected: boolean;
  disabled?: boolean;
  onClick?: () => void;
}

const OptionCard = ({ label, description, selected, disabled, onClick }: OptionCardProps) => (
  <Form.CardButton
    onClick={disabled ? undefined : onClick}
    selected={selected}
    disabled={disabled}
    sx={{ flexGrow: 1 }}
  >
    <Form.CardContent sx={{ height: "100%" }}>
      <Box display="flex" flexDirection="row" alignItems="center" height="100%" gap={1}>
        <Box>
          {selected ? <CheckCircle size={16} /> : <Circle size={16} />}
        </Box>
        <Divider orientation="vertical" flexItem />
        <Box>
          <Typography variant="h6">{label}</Typography>
          <Typography variant="caption">{description}</Typography>
        </Box>
      </Box>
    </Form.CardContent>
  </Form.CardButton>
);

const emptyParam = (): EvaluatorConfigParam => ({
  key: "",
  type: "string",
  description: "",
  required: false,
  default: undefined,
});

interface EvaluatorFormProps {
  onSubmit: (values: EvaluatorFormValues) => void;
  isSubmitting: boolean;
  serverError?: unknown;
  backHref: string;
  submitLabel: string;
  initialValues?: EvaluatorFormValues;
  isTypeEditable?: boolean;
  isLevelEditable?: boolean;
}

export function EvaluatorForm({
  onSubmit,
  isSubmitting,
  serverError,
  backHref,
  submitLabel,
  initialValues,
  isTypeEditable = true,
  isLevelEditable = true,
}: EvaluatorFormProps) {
  const [values, setValues] = useState<EvaluatorFormValues>(
    initialValues ?? defaultValues,
  );
  const [errors, setErrors] = useState<
    Partial<Record<keyof EvaluatorFormValues, string>>
  >({});
  const [page, setPage] = useState<1 | 2>(1);
  const [referenceTypeKey, setReferenceTypeKey] =
    useState<ReferenceTypeKey | null>(null);
  const [showAiPrompt, setShowAiPrompt] = useState(false);
  const [aiPromptCopied, setAiPromptCopied] = useState(false);
  const providersRegistered = useRef(false);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<Monaco | null>(null);
  const { mode: colorSchemeMode } = useColorScheme();

  // Sync form values when initialValues prop changes (e.g. after async fetch)
  useEffect(() => {
    setValues(initialValues ?? defaultValues);
  }, [initialValues]);

  // Keep module-level state in sync so Monaco providers know the current level
  useEffect(() => {
    _currentLevel = values.level;
  }, [values.level]);

  // Re-validate when level or type changes
  useEffect(() => {
    if (editorRef.current && monacoRef.current) {
      const model = editorRef.current.getModel();
      if (!model) return;
      validateFieldReferences(
        monacoRef.current,
        model,
        values.level,
        values.type === "llm_judge",
      );
    }
  }, [values.level, values.type]);

  const handleEditorBeforeMount = useCallback((monaco: Monaco) => {
    defineEditorThemes(monaco);
    if (!providersRegistered.current) {
      registerLLMJudgeLanguage(monaco);
      registerEditorProviders(monaco);
      providersRegistered.current = true;
    }
  }, []);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const handleEditorDidMount = useCallback(
    (editor: any, monaco: Monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;

      let timeout: ReturnType<typeof setTimeout>;
      const runValidation = () => {
        clearTimeout(timeout);
        timeout = setTimeout(() => {
          validateFieldReferences(
            monaco,
            editor.getModel(),
            _currentLevel,
            values.type === "llm_judge",
          );
        }, 300);
      };
      editor.onDidChangeModelContent(runValidation);
      runValidation();
    },
    [values.type],
  );

  const updateField = useCallback(
    <K extends keyof EvaluatorFormValues>(
      field: K,
      value: EvaluatorFormValues[K],
    ) => {
      setValues((prev) => ({ ...prev, [field]: value }));
      setErrors((prev) => ({ ...prev, [field]: undefined }));
    },
    [],
  );

  const handleTypeChange = useCallback(
    (newType: "code" | "llm_judge") => {
      updateField("type", newType);
      if (!initialValues) {
        updateField("source", getTemplate(newType, values.level));
      }
    },
    [initialValues, values.level, updateField],
  );

  const handleLevelChange = useCallback(
    (newLevel: EvaluatorLevel) => {
      updateField("level", newLevel);
      if (!initialValues) {
        updateField("source", getTemplate(values.type, newLevel));
      }
    },
    [initialValues, values.type, updateField],
  );

  const validate = useCallback((): boolean => {
    const newErrors: Partial<Record<keyof EvaluatorFormValues, string>> = {};
    if (!values.displayName.trim()) {
      newErrors.displayName = "Display name is required";
    }
    if (!values.source.trim()) {
      newErrors.source =
        values.type === "code"
          ? "Source code is required"
          : "Prompt template is required";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  }, [values]);

  const handleSubmit = useCallback(() => {
    if (validate()) {
      onSubmit(values);
    }
  }, [validate, onSubmit, values]);

  const canAdvance = values.displayName.trim().length > 0;

  const handleNext = useCallback(() => {
    if (!canAdvance) {
      setErrors({ displayName: "Display name is required" });
      return;
    }
    setPage(2);
  }, [canAdvance]);

  const levelOptions: {
    value: EvaluatorLevel;
    label: string;
    description: string;
  }[] = [
    {
      value: "trace",
      label: "Trace",
      description: "Evaluate the full end-to-end trace.",
    },
    {
      value: "agent",
      label: "Agent",
      description: "Evaluate each agent individually within a trace.",
    },
    {
      value: "llm",
      label: "LLM",
      description: "Evaluate individual LLM calls within a trace.",
    },
  ];

  return (
    <Form.Stack>
      {serverError ? (
        <Alert severity="error" sx={{ mb: 2 }}>
          {serverError instanceof Error
            ? serverError.message
            : "An error occurred. Please try again."}
        </Alert>
      ) : null}

      {/* ================================================================ */}
      {/* Step 1: Name, Description, Type                                  */}
      {/* ================================================================ */}
      {page === 1 && (
        <>
          <Form.Section>
            <Form.Header>Basic Details</Form.Header>
            <Form.ElementWrapper name="displayName" label="Name">
              <TextField
                id="displayName"
                placeholder="Enter evaluator name"
                value={values.displayName}
                onChange={(e) => updateField("displayName", e.target.value)}
                error={!!errors.displayName}
                helperText={
                  errors.displayName ??
                  "A human-readable name for the evaluator"
                }
                fullWidth
                required
              />
            </Form.ElementWrapper>
            <Form.ElementWrapper name="description" label="Description">
              <TextField
                id="description"
                placeholder="Describe what this evaluator checks"
                value={values.description}
                onChange={(e) => updateField("description", e.target.value)}
                multiline
                minRows={2}
                fullWidth
              />
            </Form.ElementWrapper>
          </Form.Section>

          <Form.Section>
            <Form.Header>Evaluator Type</Form.Header>
            <Box display="flex" flexDirection="row" gap={1}>
              <OptionCard
                label="Code"
                description="Write a Python function to evaluate traces programmatically."
                selected={values.type === "code"}
                disabled={!isTypeEditable}
                onClick={() => handleTypeChange("code")}
              />
              <OptionCard
                label="LLM Judge"
                description="Use an LLM to assess traces with a natural language prompt."
                selected={values.type === "llm_judge"}
                disabled={!isTypeEditable}
                onClick={() => handleTypeChange("llm_judge")}
              />
            </Box>
            {!isTypeEditable && (
              <Typography variant="caption" color="text.secondary">
                Evaluator type cannot be changed after creation.
              </Typography>
            )}
          </Form.Section>

          <Stack direction="row" spacing={2} justifyContent="space-between">
            <Button
              component={Link}
              to={backHref}
              variant="text"
              startIcon={<ArrowLeft />}
            >
              Back to Evaluators
            </Button>
            <Button
              variant="contained"
              onClick={handleNext}
              disabled={!canAdvance}
              endIcon={<ArrowRight />}
            >
              Next
            </Button>
          </Stack>
        </>
      )}

      {/* ================================================================ */}
      {/* Step 2: Level, Editor, Dependencies, Tags                        */}
      {/* ================================================================ */}
      {page === 2 && (
        <>
          <Form.Section>
            <Form.Header>
              {values.type === "code"
                ? "Evaluator Function"
                : "Evaluation Prompt"}
            </Form.Header>

            {/* Level selector */}
            <Form.ElementWrapper name="level" label="Evaluation Level">
              <Box display="flex" flexDirection="row" gap={1}>
                {levelOptions.map(({ value, label, description }) => (
                  <OptionCard
                    key={value}
                    label={label}
                    description={description}
                    selected={values.level === value}
                    disabled={!isLevelEditable}
                    onClick={() => handleLevelChange(value)}
                  />
                ))}
              </Box>
            </Form.ElementWrapper>
            {!isLevelEditable && (
              <Typography variant="caption" color="text.secondary">
                Evaluation level cannot be changed after creation.
              </Typography>
            )}

            <Divider />
            <Stack
              direction="row"
              alignItems="flex-start"
              justifyContent="space-between"
              spacing={1}
            >
              <Typography variant="body2" color="text.secondary">
                {values.type === "code" && (
                  <>
                    Write a Python function that receives a{" "}
                    {values.level === "trace" && (
                      <>
                        <Typography
                          component="span"
                          variant="body2"
                          color="primary"
                          sx={{
                            cursor: "pointer",
                            fontFamily: "monospace",
                            textDecoration: "underline",
                            textUnderlineOffset: 2,
                          }}
                          onClick={() => setReferenceTypeKey("trace")}
                        >
                          Trace
                        </Typography>{" "}
                        object and returns an{" "}
                      </>
                    )}
                    {values.level === "agent" && (
                      <>
                        <Typography
                          component="span"
                          variant="body2"
                          color="primary"
                          sx={{
                            cursor: "pointer",
                            fontFamily: "monospace",
                            textDecoration: "underline",
                            textUnderlineOffset: 2,
                          }}
                          onClick={() => setReferenceTypeKey("agent")}
                        >
                          AgentTrace
                        </Typography>{" "}
                        object and returns an{" "}
                      </>
                    )}
                    {values.level === "llm" && (
                      <>
                        <Typography
                          component="span"
                          variant="body2"
                          color="primary"
                          sx={{
                            cursor: "pointer",
                            fontFamily: "monospace",
                            textDecoration: "underline",
                            textUnderlineOffset: 2,
                          }}
                          onClick={() => setReferenceTypeKey("llm")}
                        >
                          LLMSpan
                        </Typography>{" "}
                        object and returns an{" "}
                      </>
                    )}
                    <Typography
                      component="span"
                      variant="body2"
                      color="primary"
                      sx={{
                        cursor: "pointer",
                        fontFamily: "monospace",
                        textDecoration: "underline",
                        textUnderlineOffset: 2,
                      }}
                      onClick={() => setReferenceTypeKey("eval_result")}
                    >
                      EvalResult
                    </Typography>
                    . Use{" "}
                    <Typography
                      component="span"
                      variant="body2"
                      color="primary"
                      sx={{
                        cursor: "pointer",
                        fontFamily: "monospace",
                        textDecoration: "underline",
                        textUnderlineOffset: 2,
                      }}
                      onClick={() => setReferenceTypeKey("param")}
                    >
                      Param()
                    </Typography>{" "}
                    for configurable parameters.
                  </>
                )}
                {values.type === "llm_judge" &&
                  (() => {
                    const info = LLM_JUDGE_VARIABLES[values.level];
                    return (
                      <>
                        Write a prompt template using Python f-string syntax.
                        Use{" "}
                        <Typography
                          component="span"
                          variant="body2"
                          color="primary"
                          sx={{
                            cursor: "pointer",
                            fontFamily: "monospace",
                            textDecoration: "underline",
                            textUnderlineOffset: 2,
                          }}
                          onClick={() => setReferenceTypeKey(values.level)}
                        >
                          {`{${info.varName}.*}`}
                        </Typography>{" "}
                        expressions to access{" "}
                        <Typography
                          component="span"
                          variant="body2"
                          color="primary"
                          sx={{
                            cursor: "pointer",
                            fontFamily: "monospace",
                            textDecoration: "underline",
                            textUnderlineOffset: 2,
                          }}
                          onClick={() => setReferenceTypeKey(values.level)}
                        >
                          {info.className}
                        </Typography>{" "}
                        fields. Python expressions like loops and joins are
                        supported inside{" "}
                        <Typography
                          component="span"
                          variant="body2"
                          sx={{ fontFamily: "monospace" }}
                        >
                          {"{ }"}
                        </Typography>
                        .
                      </>
                    );
                  })()}
              </Typography>

              <Button
                variant="text"
                size="small"
                startIcon={<SparklesIcon size={14} />}
                onClick={() => {
                  setShowAiPrompt(!showAiPrompt);
                  setAiPromptCopied(false);
                }}
                sx={{
                  textTransform: "none",
                  fontSize: "0.8rem",
                  whiteSpace: "nowrap",
                  flexShrink: 0,
                  mt: -0.25,
                }}
              >
                Use AI to write
              </Button>
            </Stack>
            <Collapse in={showAiPrompt}>
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
                    <Typography variant="subtitle2">
                      AI Copilot Prompt
                    </Typography>
                    <IconButton
                      size="small"
                      onClick={() => {
                        setShowAiPrompt(false);
                        setAiPromptCopied(false);
                      }}
                    >
                      <CloseIcon size={16} />
                    </IconButton>
                  </Stack>
                  <Typography variant="body2" color="text.secondary">
                    Copy this prompt and paste it into your AI assistant.
                    Describe what you want to evaluate, and the AI will generate
                    the {values.type === "code" ? "code" : "prompt"} for you.
                  </Typography>
                  <TextField
                    multiline
                    rows={8}
                    fullWidth
                    value={resolveAiPrompt(values.type, values.level)}
                    InputProps={{
                      readOnly: true,
                      sx: { fontFamily: "monospace", fontSize: "0.8rem" },
                      endAdornment: (
                        <InputAdornment
                          position="end"
                          sx={{ alignSelf: "flex-start", mt: 1, mr: -0.5 }}
                        >
                          <Tooltip
                            title={aiPromptCopied ? "Copied!" : "Copy to clipboard"}
                            placement="top"
                          >
                            <IconButton
                              size="small"
                              onClick={() => {
                                navigator.clipboard.writeText(
                                  resolveAiPrompt(values.type, values.level),
                                );
                                setAiPromptCopied(true);
                                setTimeout(() => setAiPromptCopied(false), 2000);
                              }}
                            >
                              {aiPromptCopied ? (
                                <Check size={14} />
                              ) : (
                                <Copy size={14} />
                              )}
                            </IconButton>
                          </Tooltip>
                        </InputAdornment>
                      ),
                    }}
                  />
                </Stack>
              </Box>
            </Collapse>

            <Box>
              <Box
                sx={{
                  border: 1,
                  borderColor: errors.source ? "error.main" : "divider",
                  borderRadius: 1,
                  overflow: "visible",
                  position: "relative",
                  "& .monaco-hover, & .monaco-hover *": {
                    fontSize: "11px !important",
                    lineHeight: "1.3 !important",
                  },
                }}
              >
                <Editor
                  height="400px"
                  language={
                    values.type === "llm_judge" ? LLM_JUDGE_LANG : "python"
                  }
                  theme={
                    colorSchemeMode === "dark"
                      ? EVAL_DARK_THEME
                      : EVAL_LIGHT_THEME
                  }
                  value={values.source}
                  onChange={(value) => updateField("source", value ?? "")}
                  beforeMount={handleEditorBeforeMount}
                  onMount={handleEditorDidMount}
                  options={{
                    minimap: { enabled: false },
                    scrollBeyondLastLine: false,
                    fontSize: 14,
                    lineNumbers: "on",
                    tabSize: 4,
                    automaticLayout: true,
                    hover: { above: false },
                    suggest: { showSnippets: true },
                  }}
                />
              </Box>
              {errors.source && (
                <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
                  {errors.source}
                </Typography>
              )}
            </Box>

            <Collapse in={values.type === "code"}>
              <Alert severity="info">
                <strong>Available packages:</strong>{" "}
                {SUPPORTED_PACKAGES.join(", ")}
              </Alert>
            </Collapse>
          </Form.Section>

          <Form.Section>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Box>
                <Form.Header>Config Params</Form.Header>
                <Typography variant="caption" color="text.secondary">
                  {values.type === "code"
                    ? "Params are available in your function via Param() defaults (e.g. threshold: float = Param(default=0.5))."
                    : "Params are available as {key} placeholders in your prompt template."}
                </Typography>
              </Box>
              <Button
                size="small"
                variant="outlined"
                onClick={() =>
                  updateField("configSchema", [...values.configSchema, emptyParam()])
                }
              >
                Add Param
              </Button>
            </Stack>

            <Stack spacing={1} sx={{ mt: 1 }}>
              {values.type === "llm_judge" &&
                LLM_JUDGE_BASE_CONFIG_SCHEMA.map((param) => (
                  <Box
                    key={param.key}
                    sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 1 }}
                  >
                    <Stack direction="row" spacing={1} alignItems="center">
                      <TextField
                        label="Key"
                        size="small"
                        value={param.key}
                        disabled
                        sx={{ flex: 2 }}
                        InputProps={{ sx: { fontFamily: "monospace" } }}
                      />
                      <TextField
                        label="Type"
                        size="small"
                        value={param.type}
                        disabled
                        sx={{ flex: 1 }}
                      />
                      <TextField
                        label="Default"
                        size="small"
                        value={param.default !== undefined ? String(param.default) : ""}
                        disabled
                        sx={{ flex: 1.5 }}
                      />
                      <TextField
                        label="Description"
                        size="small"
                        value={param.description}
                        disabled
                        sx={{ flex: 3 }}
                      />
                    </Stack>
                  </Box>
                ))}

              {values.configSchema.map((param, idx) => {
                const updateParam = (patch: Partial<EvaluatorConfigParam>) => {
                  const next = [...values.configSchema];
                  next[idx] = { ...next[idx], ...patch };
                  updateField("configSchema", next);
                };
                const removeParam = () => {
                  updateField(
                    "configSchema",
                    values.configSchema.filter((_, i) => i !== idx),
                  );
                };
                return (
                  <Box
                    key={idx}
                    sx={{
                      border: 1,
                      borderColor: "divider",
                      borderRadius: 1,
                      p: 1,
                    }}
                  >
                    <Stack spacing={1}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <TextField
                          label="Key"
                          size="small"
                          value={param.key}
                          onChange={(e) => updateParam({ key: e.target.value })}
                          placeholder="my_param"
                          sx={{ flex: 2 }}
                          InputProps={{ sx: { fontFamily: "monospace" } }}
                        />
                        <TextField
                          select
                          label="Type"
                          size="small"
                          value={param.type}
                          onChange={(e) =>
                            updateParam({
                              type: e.target.value,
                              enumValues: undefined,
                              min: undefined,
                              max: undefined,
                            })
                          }
                          sx={{ flex: 1 }}
                        >
                          {PARAM_TYPES.map((t) => (
                            <MenuItem key={t} value={t}>
                              {t}
                            </MenuItem>
                          ))}
                        </TextField>
                        <TextField
                          label="Default"
                          size="small"
                          value={param.default !== undefined ? String(param.default) : ""}
                          onChange={(e) =>
                            updateParam({
                              default: e.target.value === "" ? undefined : e.target.value,
                            })
                          }
                          placeholder="optional"
                          sx={{ flex: 1.5 }}
                        />
                        <TextField
                          label="Description"
                          size="small"
                          value={param.description}
                          onChange={(e) => updateParam({ description: e.target.value })}
                          placeholder="What this param controls"
                          sx={{ flex: 3 }}
                        />
                        <FormControlLabel
                          control={
                            <Checkbox
                              size="small"
                              checked={!!param.required}
                              onChange={(e) => updateParam({ required: e.target.checked })}
                            />
                          }
                          label="Required"
                          sx={{ flexShrink: 0, mr: 0 }}
                        />
                        <IconButton size="small" onClick={removeParam}>
                          <CloseIcon size={16} />
                        </IconButton>
                      </Stack>

                      {(param.type === "integer" || param.type === "float") && (
                        <Stack direction="row" spacing={1}>
                          <TextField
                            label="Min"
                            size="small"
                            type="number"
                            value={param.min !== undefined ? param.min : ""}
                            onChange={(e) =>
                              updateParam({
                                min: e.target.value === "" ? undefined : Number(e.target.value),
                              })
                            }
                            placeholder="optional"
                            sx={{ flex: 1 }}
                          />
                          <TextField
                            label="Max"
                            size="small"
                            type="number"
                            value={param.max !== undefined ? param.max : ""}
                            onChange={(e) =>
                              updateParam({
                                max: e.target.value === "" ? undefined : Number(e.target.value),
                              })
                            }
                            placeholder="optional"
                            sx={{ flex: 1 }}
                          />
                        </Stack>
                      )}

                      {param.type === "enum" && (
                        <TextField
                          label="Enum values"
                          size="small"
                          value={(param.enumValues ?? []).join(", ")}
                          onChange={(e) =>
                            updateParam({
                              enumValues: e.target.value
                                .split(",")
                                .map((s) => s.trim())
                                .filter(Boolean),
                            })
                          }
                          placeholder="value1, value2, value3"
                          fullWidth
                          helperText="Comma-separated list of allowed values"
                        />
                      )}
                    </Stack>
                  </Box>
                );
              })}
            </Stack>
          </Form.Section>

          <Form.Section>
            <Form.Header>Tags</Form.Header>
            <Autocomplete
              multiple
              freeSolo
              options={[]}
              value={values.tags}
              onChange={(_event, newValue) =>
                updateField("tags", newValue as string[])
              }
              renderTags={(tagValues, getTagProps) =>
                tagValues.map((option, index) => (
                  <Chip
                    label={option as string}
                    size="small"
                    {...getTagProps({ index })}
                    key={option as string}
                  />
                ))
              }
              renderInput={(params) => (
                <TextField {...params} placeholder="Add tags and press Enter" />
              )}
            />
          </Form.Section>

          <Stack direction="row" spacing={2} justifyContent="space-between">
            <Button
              variant="text"
              startIcon={<ArrowLeft />}
              onClick={() => setPage(1)}
            >
              Previous
            </Button>
            <Button
              variant="contained"
              color="primary"
              onClick={handleSubmit}
              disabled={isSubmitting}
            >
              {isSubmitting ? "Saving..." : submitLabel}
            </Button>
          </Stack>
        </>
      )}

      <DataModelReferenceDrawer
        open={referenceTypeKey !== null}
        onClose={() => setReferenceTypeKey(null)}
        typeKey={referenceTypeKey ?? values.level}
      />
    </Form.Stack>
  );
}
