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
import { generatePath, useParams } from "react-router-dom";
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
  ButtonGroup,
  Chip,
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

  monaco.languages.registerCompletionItemProvider("python", completionProvider);
  monaco.languages.registerHoverProvider("python", hoverProvider);
  monaco.languages.registerCompletionItemProvider(
    LLM_JUDGE_LANG,
    completionProvider,
  );
  monaco.languages.registerHoverProvider(LLM_JUDGE_LANG, hoverProvider);
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
// Python wrapper generation for LLM judge prompt templates
// ---------------------------------------------------------------------------

type SourceViewMode = "template" | "python";

const LEVEL_SIGNATURES: Record<string, { param: string; cls: string; imports: string }> = {
  trace: {
    param: "trace: Trace",
    cls: "Trace",
    imports: "from amp_evaluation.trace.models import Trace",
  },
  agent: {
    param: "agent_trace: AgentTrace",
    cls: "AgentTrace",
    imports: "from amp_evaluation.trace.models import AgentTrace",
  },
  llm: {
    param: "llm_span: LLMSpan",
    cls: "LLMSpan",
    imports: "from amp_evaluation.trace.models import LLMSpan",
  },
};

function wrapTemplateAsPython(
  template: string,
  level: string,
): string {
  const sig = LEVEL_SIGNATURES[level] ?? LEVEL_SIGNATURES.trace;
  const indented = template
    .split("\n")
    .map((line) => `        ${line}`)
    .join("\n");
  return `${sig.imports}
from amp_evaluation.dataset.models import Task
from typing import Optional


def build_prompt(self, ${sig.param}, task: Optional[Task] = None) -> str:
    return f"""
${indented}
    """
`;
}

// ---------------------------------------------------------------------------
// Config schema table
// ---------------------------------------------------------------------------

function formatDefault(value: unknown): string {
  if (value === null || value === undefined) return "-";
  if (typeof value === "boolean") return value ? "true" : "false";
  return String(value);
}

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

// ---------------------------------------------------------------------------
// Edit state
// ---------------------------------------------------------------------------

interface EditValues {
  displayName: string;
  description: string;
  source: string;
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

  const { mode: colorSchemeMode } = useColorScheme();

  const {
    data: evaluator,
    isLoading,
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
  const [sourceView, setSourceView] = useState<SourceViewMode>("template");
  const [editValues, setEditValues] = useState<EditValues>({
    displayName: "",
    description: "",
    source: "",
    tags: [],
  });

  const providersRegistered = useRef(false);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<Monaco | null>(null);

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
    setEditValues({
      displayName: evaluator.displayName,
      description: evaluator.description,
      source: evaluator.source ?? "",
      tags: evaluator.tags ?? [],
    });
    setIsEditing(true);
  }, [evaluator]);

  const handleCancelEdit = useCallback(() => {
    setIsEditing(false);
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
      registerEditorProviders(monaco);
      providersRegistered.current = true;
    }
  }, []);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const handleEditorDidMount = useCallback(
    (editor: any, monaco: Monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;

      if (isEditing && evaluator) {
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
        editor.onDidChangeModelContent(runValidation);
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
  const isCustomLLMJudge = isLLMJudge && !evaluator.isBuiltin;
  const showViewToggle = isCustomLLMJudge && !isEditing;
  const sourceLabel = isLLMJudge
    ? sourceView === "python"
      ? "Python Function"
      : "Prompt Template"
    : "Source Code";
  const editorLanguage =
    isLLMJudge && sourceView !== "python" ? LLM_JUDGE_LANG : "python";

  const editorTheme = isEditing
    ? colorSchemeMode === "dark"
      ? EVAL_DARK_THEME
      : EVAL_LIGHT_THEME
    : colorSchemeMode === "dark"
      ? VIEW_DARK_THEME
      : VIEW_LIGHT_THEME;

  const rawSource = isEditing ? editValues.source : (evaluator.source ?? "");
  const source =
    isLLMJudge && sourceView === "python" && !isEditing
      ? wrapTemplateAsPython(rawSource, evaluator.level)
      : rawSource;
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
              variant="filled"
              color={evaluator.isBuiltin ? "default" : "info"}
            />
            {evaluator.type && (
              <Chip
                label={isLLMJudge ? "LLM Judge" : "Code"}
                color={isLLMJudge ? "warning" : "info"}
                size="small"
              />
            )}
            <Chip
              label={
                evaluator.level.charAt(0).toUpperCase() +
                evaluator.level.slice(1)
              }
              variant="outlined"
              color="primary"
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
              variant="filled"
              color={evaluator.isBuiltin ? "default" : "info"}
            />
            {evaluator.type && (
              <Chip
                label={isLLMJudge ? "LLM Judge" : "Code"}
                color={isLLMJudge ? "warning" : "info"}
                size="small"
              />
            )}
            <Chip
              label={
                evaluator.level.charAt(0).toUpperCase() +
                evaluator.level.slice(1)
              }
              variant="outlined"
              color="primary"
              size="small"
            />
          </Stack>
        )}

        {/* Display Name */}
        {isEditing && (
          <TextField
            label="Name"
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
        )}

        {/* Description */}
        {isEditing ? (
          <TextField
            label="Description"
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
        ) : null}

        {/* Source code / prompt */}
        {source && (
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
                ...(isEditing && {
                  "& .monaco-hover, & .monaco-hover *": {
                    fontSize: "11px !important",
                    lineHeight: "1.3 !important",
                  },
                }),
              }}
            >
              {showViewToggle && (
                <ButtonGroup
                  size="small"
                  sx={{
                    position: "absolute",
                    top: 8,
                    right: 16,
                    zIndex: 2,
                  }}
                >
                  <Button
                    variant={sourceView === "template" ? "contained" : "outlined"}
                    onClick={() => setSourceView("template")}
                    sx={{ textTransform: "none", fontSize: 12, py: 0.25, px: 1 }}
                  >
                    Prompt
                  </Button>
                  <Button
                    variant={sourceView === "python" ? "contained" : "outlined"}
                    onClick={() => setSourceView("python")}
                    sx={{ textTransform: "none", fontSize: 12, py: 0.25, px: 1 }}
                  >
                    Python
                  </Button>
                </ButtonGroup>
              )}
              <Editor
                height="400px"
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

        {/* Config schema — always read-only */}
        <ConfigSchemaTable configSchema={evaluator.configSchema} />
      </Stack>
    </PageLayout>
  );
};
