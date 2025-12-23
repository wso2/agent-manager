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

import { NoDataFound, TextInput } from "@agent-management-platform/views";
import Editor, { type Monaco } from "@monaco-editor/react";
import {
  Stack,
  useColorScheme,
  Typography,
  IconButton,
} from "@wso2/oxygen-ui";
import {
  ChartArea,
  ChevronUp,
  ChevronDown,
  Search,
} from "@wso2/oxygen-ui-icons-react";
import { useState, useRef } from "react";

interface AttributesSectionProps {
  attributes?: Record<string, unknown>;
}

const CUSTOM_DARK_THEME = "custom-dark-transparent";
const CUSTOM_LIGHT_THEME = "custom-light-transparent";

function defineCustomThemes(monaco: Monaco) {
  // Define custom dark theme with transparent background
  monaco.editor.defineTheme(CUSTOM_DARK_THEME, {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": "#00000000", // Transparent
    },
  });

  // Define custom light theme with transparent background
  monaco.editor.defineTheme(CUSTOM_LIGHT_THEME, {
    base: "vs",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": "#00000000", // Transparent
    },
  });
}

type MatchRange = {
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
};

type EditorInstance = Parameters<
  NonNullable<React.ComponentProps<typeof Editor>["onMount"]>
>[0];

/**
 * Safely stringifies attributes, handling circular references and unsupported types
 */
function safeStringifyAttributes(
  attributes: Record<string, unknown>
): string {
  const seen = new WeakSet();

  const replacer = (_key: string, value: unknown): unknown => {
    // Handle BigInt values
    if (typeof value === "bigint") {
      return value.toString();
    }

    // Handle circular references
    if (typeof value === "object" && value !== null) {
      if (seen.has(value)) {
        return "[Circular]";
      }
      seen.add(value);
    }

    // Handle other unsupported types
    if (value === undefined) {
      return "[undefined]";
    }

    return value;
  };

  try {
    return JSON.stringify(attributes, replacer, 2);
  } catch {
    // Fallback: attempt shallow serialization
    try {
      const shallowCopy: Record<string, unknown> = {};
      for (const [key, val] of Object.entries(attributes)) {
        try {
          if (typeof val === "bigint") {
            shallowCopy[key] = val.toString();
          } else if (val === undefined) {
            shallowCopy[key] = "[undefined]";
          } else {
            shallowCopy[key] = JSON.parse(JSON.stringify(val, replacer));
          }
        } catch {
          shallowCopy[key] = "[unserializable]";
        }
      }
      return JSON.stringify(shallowCopy, null, 2);
    } catch {
      // Final fallback
      return "<unserializable attributes>";
    }
  }
}

export function AttributesSection({ attributes }: AttributesSectionProps) {
  const { mode: colorSchemeMode } = useColorScheme();
  const [searchText, setSearchText] = useState("");
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const [totalMatches, setTotalMatches] = useState(0);
  const editorRef = useRef<EditorInstance | null>(null);
  const matchesRef = useRef<Array<{ range: MatchRange }>>([]);

  const handleEditorWillMount = (monaco: Monaco) => {
    defineCustomThemes(monaco);
  };

  const handleEditorMount = (editor: EditorInstance) => {
    editorRef.current = editor;
  };

  const navigateToMatch = (index: number) => {
    if (!editorRef.current || matchesRef.current.length === 0) return;

    const match = matchesRef.current[index];
    if (match) {
      editorRef.current.setPosition({
        lineNumber: match.range.startLineNumber,
        column: match.range.startColumn,
      });
      editorRef.current.revealLineInCenter(match.range.startLineNumber);
      editorRef.current.setSelection(match.range);
      setCurrentMatchIndex(index + 1);
    }
  };

  const performSearch = (searchValue: string) => {
    if (!editorRef.current || !searchValue.trim()) {
      matchesRef.current = [];
      setTotalMatches(0);
      setCurrentMatchIndex(0);
      return;
    }

    const model = editorRef.current.getModel();
    if (!model) return;

    const matches: Array<{ range: MatchRange }> = [];
    const escapedValue = searchValue.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const searchRegex = new RegExp(escapedValue, "gi");
    const text = model.getValue();

    let match;
    while ((match = searchRegex.exec(text)) !== null) {
      const position = model.getPositionAt(match.index);
      const endPosition = model.getPositionAt(match.index + match[0].length);

      matches.push({
        range: {
          startLineNumber: position.lineNumber,
          startColumn: position.column,
          endLineNumber: endPosition.lineNumber,
          endColumn: endPosition.column,
        },
      });
    }

    matchesRef.current = matches;
    setTotalMatches(matches.length);

    if (matches.length > 0) {
      setCurrentMatchIndex(1);
      navigateToMatch(0);
    } else {
      setCurrentMatchIndex(0);
    }
  };

  const handleSearchChange = (value: string) => {
    setSearchText(value);
    performSearch(value);
  };

  const handleNext = () => {
    if (matchesRef.current.length === 0) return;
    const nextIndex = currentMatchIndex % matchesRef.current.length;
    navigateToMatch(nextIndex);
  };

  const handlePrevious = () => {
    if (matchesRef.current.length === 0) return;
    const prevIndex =
      currentMatchIndex <= 1
        ? matchesRef.current.length - 1
        : currentMatchIndex - 2;
    navigateToMatch(prevIndex);
  };

  if (!attributes || Object.keys(attributes).length === 0) {
    return (
      <NoDataFound
        message="No attributes found"
        iconElement={ChartArea}
        subtitle="Try selecting a different span to view attributes"
        disableBackground
      />
    );
  }

  return (
    <Stack direction="column" spacing={2}>
      <Stack
        spacing={1}
        pt={2}
        direction="row"
        justifyContent="right"
        alignItems="center"
      >
        <TextInput
          placeholder="Search..."
          value={searchText}
          onChange={(e) => handleSearchChange(e.target.value)}
          size="small"
          fullWidth
          aria-label="Search attributes"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              handleNext();
            }
          }}
          slotProps={{
            input: {
              endAdornment:
                totalMatches > 0 ? (
                  <Stack direction="row" alignItems="center" gap={1}>
                    <Typography variant="caption" noWrap>
                      {currentMatchIndex} / {totalMatches}
                    </Typography>
                  </Stack>
                ) : (
                  <Search size={16} aria-hidden="true" />
                ),
            },
          }}
        />

        {totalMatches > 0 && (
          <Stack direction="row" alignItems="center" gap={1}>
            <IconButton
              size="small"
              onClick={handlePrevious}
              aria-label="Previous match"
              title="Previous match"
            >
              <ChevronUp size={16} aria-hidden="true" />
            </IconButton>
            <IconButton
              size="small"
              onClick={handleNext}
              aria-label="Next match"
              title="Next match"
            >
              <ChevronDown size={16} aria-hidden="true" />
            </IconButton>
          </Stack>
        )}
      </Stack>
      <Editor
        height="calc(100vh - 266px)"
        theme={
          colorSchemeMode === "dark" ? CUSTOM_DARK_THEME : CUSTOM_LIGHT_THEME
        }
        value={safeStringifyAttributes(attributes)}
        language="json"
        beforeMount={handleEditorWillMount}
        onMount={handleEditorMount}
        options={{
          readOnly: true,
          minimap: {
            enabled: false,
          },
          scrollBeyondLastLine: false,
          scrollbar: {
            horizontal: "auto",
            vertical: "auto",
          },
          wordWrap: "on",
          folding: true,
          foldingStrategy: "auto",
          showFoldingControls: "always",
          foldingHighlight: true,
          unfoldOnClickAfterEndOfLine: true,
        }}
      />
    </Stack>
  );
}
