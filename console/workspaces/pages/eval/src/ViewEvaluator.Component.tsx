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

import React, { useCallback, useRef, useState, useEffect } from "react";
import { generatePath, useLocation, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type EvaluatorConfigParam,
  type EvaluatorLevel,
  type UpdateCustomEvaluatorRequest,
} from "@agent-management-platform/types";
import {
  useGetEvaluator,
  useUpdateCustomEvaluator,
} from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  FormControlLabel,
  IconButton,
  MenuItem,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  useColorScheme,
} from "@wso2/oxygen-ui";
import { Pencil, X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import Editor, { type Monaco } from "@monaco-editor/react";
import {
  COMPLETIONS,
  COMMON_COMPLETIONS,
  HOVER_DOCS,
  LLM_JUDGE_VARIABLES,
  type CompletionSuggestion,
} from "./generated/evaluator-models.generated";
import {
  generateCodeHeader,
  extractCodeBody,
} from "./subComponents/EvaluatorForm";
import { SectionErrorBoundary } from "./subComponents/SectionErrorBoundary";

// ---------------------------------------------------------------------------
// Monaco setup — reuse patterns from EvaluatorForm
// ---------------------------------------------------------------------------

const LLM_JUDGE_LANG = "llm-judge-prompt";
const EVAL_DARK_THEME = "eval-dark";
const EVAL_LIGHT_THEME = "eval-light";
const VIEW_DARK_THEME = "view-dark";
const VIEW_LIGHT_THEME = "view-light";

let _currentLevel: EvaluatorLevel = "trace";

function registerLLMJudgeLanguage(monaco: Monaco) {
  monaco.languages.register({ id: LLM_JUDGE_LANG });
  monaco.languages.setMonarchTokensProvider(LLM_JUDGE_LANG, {
    tokenizer: {
      root: [
        [/\{\{/, "string"],
        [/\}\}/, "string"],
        [/\{/, { token: "delimiter.bracket", next: "@fstring" }],
        [/./, "string"],
      ],
      fstring: [
        [/\{/, { token: "delimiter.bracket", next: "@fstring" }],
        [/\}/, { token: "delimiter.bracket", next: "@pop" }],
        [/"[^"]*"/, "string.python"],
        [/'[^']*'/, "string.python"],
        [/\b\d+(\.\d+)?\b/, "number"],
        [/[a-zA-Z_]\w*(?=\()/, "identifier.method"],
        [/\./, "delimiter"],
        [/[a-zA-Z_]\w*/, "identifier"],
        [/[,()[\]:+\-*/%=<>!&|~^]/, "delimiter"],
        [/\s+/, "white"],
      ],
    },
  });
}

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

  return [
    monaco.languages.registerCompletionItemProvider("python", completionProvider),
    monaco.languages.registerHoverProvider("python", hoverProvider),
    monaco.languages.registerCompletionItemProvider(
      LLM_JUDGE_LANG,
      completionProvider,
    ),
    monaco.languages.registerHoverProvider(LLM_JUDGE_LANG, hoverProvider),
  ];
}

function buildValidFields(): Record<EvaluatorLevel, Set<string>> {
  const result: Record<string, Set<string>> = {};
  for (const level of ["trace", "agent", "llm"] as const) {
    const fields = new Set<string>();
    for (const item of COMPLETIONS[level]) {
      if (item.label.includes(".")) {
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
  const pattern = new RegExp(`(?<![.\\w])${rootVar}\\.([a-zA-Z_]\\w*)`, "g");

  let match;
  while ((match = pattern.exec(text)) !== null) {
    const fullRef = `${rootVar}.${match[1]}`;
    if (validFields.has(fullRef)) continue;

    if (isLLMJudge) {
      const before = text.slice(0, match.index);
      const openBraces = (before.match(/{/g) || []).length;
      const closeBraces = (before.match(/}/g) || []).length;
      if (openBraces <= closeBraces) continue;
    }

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

function defineThemes(monaco: Monaco) {
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
  monaco.editor.defineTheme(VIEW_DARK_THEME, {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: {},
  });
  monaco.editor.defineTheme(VIEW_LIGHT_THEME, {
    base: "vs",
    inherit: true,
    rules: [],
    colors: {},
  });
}

// ---------------------------------------------------------------------------
// Config schema table
// ---------------------------------------------------------------------------

function formatDefault(value: unknown): string {
  if (value === null || value === undefined) return "-";
  if (typeof value === "boolean") return value ? "true" : "false";
  return String(value);
}

const PARAM_TYPES = ["string", "integer", "float", "boolean", "array", "enum"] as const;

const emptyParam = (): EvaluatorConfigParam => ({
  key: "",
  type: "string",
  description: "",
  required: false,
  default: undefined,
});

function ConfigSchemaTable({
  configSchema,
}: {
  configSchema: EvaluatorConfigParam[];
}) {
  if (!configSchema || configSchema.length === 0) return null;

  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        Configuration Parameters
      </Typography>
      <TableContainer
        sx={{ border: 1, borderColor: "divider", borderRadius: 1 }}
      >
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Parameter</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Default</TableCell>
              <TableCell>Required</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {configSchema.map((param) => (
              <TableRow key={param.key}>
                <TableCell>
                  <Typography variant="body2" fontFamily="monospace">
                    {param.key}
                  </Typography>
                </TableCell>
                <TableCell>{param.type}</TableCell>
                <TableCell>{param.description}</TableCell>
                <TableCell>
                  <Typography variant="body2" fontFamily="monospace">
                    {formatDefault(param.default)}
                  </Typography>
                </TableCell>
                <TableCell>{param.required ? "Yes" : "No"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}

function EditableConfigParams({
  configSchema,
  onChange,
  evaluatorType,
}: {
  configSchema: EvaluatorConfigParam[];
  onChange: (params: EvaluatorConfigParam[]) => void;
  evaluatorType: string;
}) {
  const updateParam = (idx: number, patch: Partial<EvaluatorConfigParam>) => {
    const next = [...configSchema];
    next[idx] = { ...next[idx], ...patch };
    onChange(next);
  };

  const removeParam = (idx: number) => {
    onChange(configSchema.filter((_, i) => i !== idx));
  };

  return (
    <Box>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
      >
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Configuration Parameters
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {evaluatorType === "code"
              ? "Params are available in your function via Param() defaults (e.g. threshold: float = Param(default=0.5))."
              : "Params are available as {key} placeholders in your prompt template."}
          </Typography>
        </Box>
        <Button
          size="small"
          variant="outlined"
          onClick={() => onChange([...configSchema, emptyParam()])}
        >
          Add Param
        </Button>
      </Stack>

      <Stack spacing={1} sx={{ mt: 1 }}>
        {configSchema.map((param, idx) => (
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
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                <TextField
                  placeholder="Key"
                  size="small"
                  value={param.key}
                  onChange={(e) => updateParam(idx, { key: e.target.value })}
                  sx={{ flex: 2, minWidth: 120 }}
                  InputProps={{ sx: { fontFamily: "monospace" } }}
                />
                <TextField
                  select
                  placeholder="Type"
                  size="small"
                  value={param.type}
                  onChange={(e) =>
                    updateParam(idx, {
                      type: e.target.value,
                      enumValues: undefined,
                      min: undefined,
                      max: undefined,
                    })
                  }
                  sx={{ flex: 1, minWidth: 80 }}
                >
                  {PARAM_TYPES.map((t) => (
                    <MenuItem key={t} value={t}>
                      {t}
                    </MenuItem>
                  ))}
                </TextField>
                <TextField
                  placeholder="Default"
                  size="small"
                  value={param.default !== undefined ? String(param.default) : ""}
                  onChange={(e) =>
                    updateParam(idx, {
                      default: e.target.value === "" ? undefined : e.target.value,
                    })
                  }
                  sx={{ flex: 1.5, minWidth: 100 }}
                />
                <TextField
                  placeholder="Description"
                  size="small"
                  value={param.description}
                  onChange={(e) => updateParam(idx, { description: e.target.value })}
                  sx={{ flex: 3, minWidth: 150 }}
                />
                <FormControlLabel
                  control={
                    <Checkbox
                      size="small"
                      checked={!!param.required}
                      onChange={(e) => updateParam(idx, { required: e.target.checked })}
                    />
                  }
                  label="Required"
                  sx={{ flexShrink: 0, mr: 0 }}
                />
                <IconButton size="small" onClick={() => removeParam(idx)}>
                  <CloseIcon size={16} />
                </IconButton>
              </Stack>

              {(param.type === "integer" || param.type === "float") && (
                <Stack direction="row" spacing={1}>
                  <TextField
                    placeholder="Min"
                    size="small"
                    type="number"
                    value={param.min !== undefined ? param.min : ""}
                    onChange={(e) =>
                      updateParam(idx, {
                        min: e.target.value === "" ? undefined : Number(e.target.value),
                      })
                    }
                    sx={{ flex: 1 }}
                  />
                  <TextField
                    placeholder="Max"
                    size="small"
                    type="number"
                    value={param.max !== undefined ? param.max : ""}
                    onChange={(e) =>
                      updateParam(idx, {
                        max: e.target.value === "" ? undefined : Number(e.target.value),
                      })
                    }
                    sx={{ flex: 1 }}
                  />
                </Stack>
              )}

              {param.type === "enum" && (
                <TextField
                  placeholder="value1, value2, value3"
                  size="small"
                  value={(param.enumValues ?? []).join(", ")}
                  onChange={(e) =>
                    updateParam(idx, {
                      enumValues: e.target.value
                        .split(",")
                        .map((s) => s.trim())
                        .filter(Boolean),
                    })
                  }
                  fullWidth
                />
              )}
            </Stack>
          </Box>
        ))}
      </Stack>
    </Box>
  );
}

// ---------------------------------------------------------------------------
// Edit state
// ---------------------------------------------------------------------------

interface EditValues {
  displayName: string;
  description: string;
  source: string;
  configSchema: EvaluatorConfigParam[];
  tags: string[];
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export const ViewEvaluatorComponent: React.FC = () => {
  const { agentId, orgId, projectId, evaluatorId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    evaluatorId: string;
  }>();

  const location = useLocation();
  const { mode: colorSchemeMode } = useColorScheme();

  const {
    data: evaluator,
    isLoading,
    error: fetchError,
    refetch,
  } = useGetEvaluator({
    orgName: orgId!,
    evaluatorId: evaluatorId!,
  });

  const {
    mutate: updateEvaluator,
    isPending: isSaving,
    error: saveError,
  } = useUpdateCustomEvaluator({
    orgName: orgId!,
    identifier: evaluatorId!,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [editValues, setEditValues] = useState<EditValues>({
    displayName: "",
    description: "",
    source: "",
    configSchema: [],
    tags: [],
  });

  const providersRegistered = useRef(false);
  const providerDisposablesRef = useRef<{ dispose(): void }[]>([]);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<Monaco | null>(null);
  const validationCleanupRef = useRef<(() => void) | null>(null);

  // --- Code evaluator: auto-generated header tracking (edit mode) ---
  const editSourceRef = useRef(editValues.source);
  editSourceRef.current = editValues.source;

  // Sync configSchema → code editor header when editing a code evaluator
  useEffect(() => {
    if (!isEditing || !evaluator || evaluator.type !== "code") return;

    let expectedHeader: string;
    try {
      expectedHeader = generateCodeHeader(
        evaluator.level,
        editValues.configSchema,
      );
    } catch {
      // Param keys may be temporarily invalid while the user is typing
      return;
    }

    // If the source already starts with the correct header, nothing to do
    if (editSourceRef.current.startsWith(expectedHeader + "\n")) return;

    // Regenerate: keep existing body, replace header
    const body = extractCodeBody(editSourceRef.current);
    const newSource = expectedHeader + "\n" + body;
    if (newSource === editSourceRef.current) return;

    setEditValues((prev) => ({ ...prev, source: newSource }));
    editSourceRef.current = newSource;
  }, [isEditing, evaluator, editValues.configSchema, editValues.source]);

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents.children
      .evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  const backHref = generatePath(evaluatorsRouteMap.path, routeParams);

  // Sync _currentLevel for Monaco providers
  useEffect(() => {
    if (evaluator) {
      _currentLevel = evaluator.level;
    }
  }, [evaluator]);

  // Clean up providers and validation listener on unmount
  useEffect(() => {
    return () => {
      validationCleanupRef.current?.();
      for (const d of providerDisposablesRef.current) {
        d.dispose();
      }
      providerDisposablesRef.current = [];
      providersRegistered.current = false;
    };
  }, []);

  // Re-validate when entering edit mode
  useEffect(() => {
    if (isEditing && editorRef.current && monacoRef.current && evaluator) {
      validateFieldReferences(
        monacoRef.current,
        editorRef.current.getModel(),
        evaluator.level,
        evaluator.type === "llm_judge",
      );
    }
  }, [isEditing, evaluator]);

  const handleStartEdit = useCallback(() => {
    if (!evaluator) return;
    // For LLM judge, the API response prepends base config params (model, temperature, etc.)
    // Strip them so the user only edits their custom params.
    const baseKeys = new Set(["model", "temperature", "max_tokens", "max_retries"]);
    const configSchema = evaluator.configSchema
      ? evaluator.type === "llm_judge"
        ? evaluator.configSchema.filter((p) => !baseKeys.has(p.key))
        : [...evaluator.configSchema]
      : [];
    let source = evaluator.source ?? "";

    // For code evaluators, ensure the source has the auto-generated header
    if (evaluator.type === "code" && source) {
      const expectedHeader = generateCodeHeader(evaluator.level, configSchema);
      if (!source.startsWith(expectedHeader + "\n")) {
        const body = extractCodeBody(source);
        source = expectedHeader + "\n" + body;
      }
    }

    setEditValues({
      displayName: evaluator.displayName,
      description: evaluator.description,
      source,
      configSchema,
      tags: evaluator.tags ?? [],
    });
    setIsEditing(true);
  }, [evaluator]);

  useEffect(() => {
    if (location.state?.edit && evaluator && !isEditing) {
      handleStartEdit();
    }
  }, [location.state, evaluator, isEditing, handleStartEdit]);

  const handleCancelEdit = useCallback(() => {
    setIsEditing(false);
    // Clean up validation listener
    validationCleanupRef.current?.();
    validationCleanupRef.current = null;
    // Clear validation markers
    if (editorRef.current && monacoRef.current) {
      monacoRef.current.editor.setModelMarkers(
        editorRef.current.getModel(),
        "evaluator-validation",
        [],
      );
    }
  }, []);

  const handleSave = useCallback(() => {
    if (!editValues.displayName.trim()) return;
    const body: UpdateCustomEvaluatorRequest = {
      displayName: editValues.displayName,
      description: editValues.description,
      source: editValues.source,
      configSchema: editValues.configSchema,
      tags: editValues.tags,
    };
    updateEvaluator(body, {
      onSuccess: () => {
        setIsEditing(false);
        refetch();
      },
    });
  }, [editValues, updateEvaluator, refetch]);

  const handleEditorBeforeMount = useCallback((monaco: Monaco) => {
    defineThemes(monaco);
    if (!providersRegistered.current) {
      registerLLMJudgeLanguage(monaco);
      providerDisposablesRef.current = registerEditorProviders(monaco);
      providersRegistered.current = true;
    }
  }, []);

   
  const handleEditorDidMount = useCallback(
    (editor: any, monaco: Monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;

      if (isEditing && evaluator) {
        // Clean up any previous listener
        validationCleanupRef.current?.();

        let timeout: ReturnType<typeof setTimeout>;
        const runValidation = () => {
          clearTimeout(timeout);
          timeout = setTimeout(() => {
            validateFieldReferences(
              monaco,
              editor.getModel(),
              evaluator.level,
              evaluator.type === "llm_judge",
            );
          }, 300);
        };
        const disposable = editor.onDidChangeModelContent(runValidation);
        validationCleanupRef.current = () => {
          clearTimeout(timeout);
          disposable?.dispose();
        };
        runValidation();
      }
    },
    [isEditing, evaluator],
  );

  if (isLoading) {
    return (
      <PageLayout title="Evaluator" disableIcon>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  if (fetchError) {
    return (
      <PageLayout
        title="Evaluator"
        backHref={backHref}
        backLabel="Back to Evaluators"
        disableIcon
      >
        <Alert severity="error">Failed to load evaluator. Please try again.</Alert>
      </PageLayout>
    );
  }

  if (!evaluator) {
    return (
      <PageLayout
        title="Evaluator"
        backHref={backHref}
        backLabel="Back to Evaluators"
        disableIcon
      >
        <Typography>Evaluator not found.</Typography>
      </PageLayout>
    );
  }

  const isLLMJudge = evaluator.type === "llm_judge";
  const sourceLabel = isLLMJudge ? "Prompt Template" : "Source Code";
  const editorLanguage = isLLMJudge ? LLM_JUDGE_LANG : "python";

  const editorTheme = isEditing
    ? colorSchemeMode === "dark"
      ? EVAL_DARK_THEME
      : EVAL_LIGHT_THEME
    : colorSchemeMode === "dark"
      ? VIEW_DARK_THEME
      : VIEW_LIGHT_THEME;

  const source = isEditing ? editValues.source : (evaluator.source ?? "");
  const tags = isEditing ? editValues.tags : (evaluator.tags ?? []);

  return (
    <PageLayout
      title={isEditing ? "Edit Evaluator" : evaluator.displayName}
      description={!isEditing ? evaluator.description : undefined}
      backHref={backHref}
      backLabel="Back to Evaluators"
      disableIcon
      titleTail={
        !isEditing ? (
          <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
            <Chip
              label={evaluator.isBuiltin ? "Built-in" : "Custom"}
              size="small"
              variant="outlined"
              color={evaluator.isBuiltin ? "default" : "info"}
            />
            {evaluator.type && (
              <Chip
                label={isLLMJudge ? "LLM Judge" : "Code"}
                variant="outlined"
                size="small"
              />
            )}
            <Chip
              label={
                evaluator.level.charAt(0).toUpperCase() +
                evaluator.level.slice(1)
              }
              variant="outlined"
              size="small"
            />
          </Stack>
        ) : undefined
      }
      actions={
        <Stack direction="row" spacing={1}>
          {!evaluator.isBuiltin && !isEditing && (
            <Button
              variant="contained"
              startIcon={<Pencil size={16} />}
              onClick={handleStartEdit}
            >
              Edit
            </Button>
          )}
          {isEditing && (
            <>
              <Button
                variant="text"
                startIcon={<CloseIcon size={16} />}
                onClick={handleCancelEdit}
                disabled={isSaving}
              >
                Cancel
              </Button>
              <Button
                variant="contained"
                onClick={handleSave}
                disabled={isSaving || !editValues.displayName.trim()}
              >
                {isSaving ? "Saving..." : "Save"}
              </Button>
            </>
          )}
        </Stack>
      }
    >
      <Stack spacing={3}>
        {!!saveError && (
          <Alert severity="error">
            {saveError instanceof Error
              ? saveError.message
              : "Failed to save. Please try again."}
          </Alert>
        )}

        {/* Metadata chips — shown in body only during editing */}
        {isEditing && (
          <Stack direction="row" spacing={1} flexWrap="wrap">
            <Chip
              label={evaluator.isBuiltin ? "Built-in" : "Custom"}
              size="small"
              variant="outlined"
              color={evaluator.isBuiltin ? "default" : "info"}
            />
            {evaluator.type && (
              <Chip
                label={isLLMJudge ? "LLM Judge" : "Code"}
                variant="outlined"
                size="small"
              />
            )}
            <Chip
              label={
                evaluator.level.charAt(0).toUpperCase() +
                evaluator.level.slice(1)
              }
              variant="outlined"
              size="small"
            />
          </Stack>
        )}

        {/* Display Name */}
        {isEditing && (
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Name
            </Typography>
            <TextField
              placeholder="Enter evaluator name"
              value={editValues.displayName}
              onChange={(e) =>
                setEditValues((prev) => ({
                  ...prev,
                  displayName: e.target.value,
                }))
              }
              error={!editValues.displayName.trim()}
              helperText={
                !editValues.displayName.trim() ? "Name is required" : undefined
              }
              fullWidth
              required
            />
          </Box>
        )}

        {/* Description */}
        {isEditing ? (
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Description
            </Typography>
            <TextField
              placeholder="Describe what this evaluator checks"
              value={editValues.description}
              onChange={(e) =>
                setEditValues((prev) => ({
                  ...prev,
                  description: e.target.value,
                }))
              }
              multiline
              minRows={2}
              fullWidth
            />
          </Box>
        ) : null}

        {/* Source code / prompt */}
        {source && (
          <SectionErrorBoundary fallbackMessage="The code editor failed to load. Click Retry to try again.">
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                {sourceLabel}
              </Typography>
              <Box
                sx={{
                  border: 1,
                  borderColor: "divider",
                  borderRadius: 1,
                  overflow: isEditing ? "visible" : "hidden",
                  position: "relative",
                  minHeight: 300,
                  height: "calc(100vh - 500px)",
                  ...(isEditing && {
                    "& .monaco-hover, & .monaco-hover *": {
                      fontSize: "11px !important",
                      lineHeight: "1.3 !important",
                    },
                  }),
                }}
              >
                <Editor
                  height="100%"
                  language={editorLanguage}
                  theme={editorTheme}
                  value={source}
                  onChange={
                    isEditing
                      ? (value) =>
                        setEditValues((prev) => ({
                          ...prev,
                          source: value ?? "",
                        }))
                      : undefined
                  }
                  beforeMount={handleEditorBeforeMount}
                  onMount={handleEditorDidMount}
                  options={{
                    readOnly: !isEditing,
                    minimap: { enabled: false },
                    scrollBeyondLastLine: false,
                    fontSize: 14,
                    lineNumbers: "on",
                    tabSize: 4,
                    automaticLayout: true,
                    ...(isEditing && {
                      hover: { above: false },
                      suggest: { showSnippets: true },
                    }),
                  }}
                />
              </Box>
            </Box>
          </SectionErrorBoundary>
        )}

        {/* Tags */}
        {isEditing ? (
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Tags
            </Typography>
            <Autocomplete
              multiple
              freeSolo
              options={[]}
              value={editValues.tags}
              onChange={(_event, newValue) =>
                setEditValues((prev) => ({
                  ...prev,
                  tags: newValue as string[],
                }))
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
          </Box>
        ) : (
          tags.length > 0 && (
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Tags
              </Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap">
                {tags.map((tag) => (
                  <Chip key={tag} label={tag} size="small" variant="outlined" />
                ))}
              </Stack>
            </Box>
          )
        )}

        {/* Config schema */}
        <SectionErrorBoundary fallbackMessage="Config parameters section failed to render. Click Retry to try again.">
          {isEditing ? (
            <EditableConfigParams
              configSchema={editValues.configSchema}
              onChange={(params) =>
                setEditValues((prev) => ({ ...prev, configSchema: params }))
              }
              evaluatorType={evaluator.type ?? "code"}
            />
          ) : (
            <ConfigSchemaTable configSchema={evaluator.configSchema} />
          )}
        </SectionErrorBoundary>
      </Stack>
    </PageLayout>
  );
};
