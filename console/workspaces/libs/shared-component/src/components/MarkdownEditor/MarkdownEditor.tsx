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

import { useEffect, useId, useRef, useState } from "react";
import { Box, FormHelperText, FormLabel, IconButton, Stack, Tab, Tabs, TextField, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Bold, Heading2, Italic, Link2, List, Quote } from "@wso2/oxygen-ui-icons-react";
import { MarkdownView } from "@agent-management-platform/views";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import { type EditResult, type TextSelection, insertLink, prefixLines, wrapSelection } from "./textEditActions";

type MarkdownEditorTab = "write" | "preview";

const TOOLBAR_ACTIONS = [
  {
    key: "heading",
    label: "Heading",
    Icon: Heading2,
    apply: (sel: TextSelection, end: number) => prefixLines(sel, end, "### "),
  },
  {
    key: "bold",
    label: "Bold",
    Icon: Bold,
    apply: (sel: TextSelection, end: number) => wrapSelection(sel, end, "**", "**", "bold text"),
  },
  {
    key: "italic",
    label: "Italic",
    Icon: Italic,
    apply: (sel: TextSelection, end: number) => wrapSelection(sel, end, "*", "*", "italic text"),
  },
  {
    key: "quote",
    label: "Quote",
    Icon: Quote,
    apply: (sel: TextSelection, end: number) => prefixLines(sel, end, "> "),
  },
  {
    key: "list",
    label: "Bulleted list",
    Icon: List,
    apply: (sel: TextSelection, end: number) => prefixLines(sel, end, "- "),
  },
  {
    key: "link",
    label: "Link",
    Icon: Link2,
    apply: insertLink,
  },
] as const;

export interface MarkdownEditorProps {
  id?: string;
  label?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  error?: boolean;
  helperText?: React.ReactNode;
  disabled?: boolean;
  minRows?: number;
  maxRows?: number;
  /**
   * Character cap for the document. Defaults to the shared description limit
   * because every current caller is a description field. The cap exists
   * because the value is submitted inside a JSON request body, and the WAF in
   * front of the platform rejects an oversized body with a bare 403 that the
   * console cannot attribute to a field.
   */
  maxLength?: number;
}

/**
 * A GitHub-style markdown input: a formatting toolbar (heading, bold, italic,
 * quote, list, link) and a "Write" tab with a plain textarea, plus a
 * "Preview" tab that renders the same content through MarkdownView.
 */
export const MarkdownEditor = ({
  id,
  label,
  value,
  onChange,
  placeholder,
  error = false,
  helperText,
  disabled = false,
  minRows = 3,
  maxRows = 10,
  maxLength = INPUT_LIMITS.DESCRIPTION,
}: MarkdownEditorProps) => {
  const [tab, setTab] = useState<MarkdownEditorTab>("write");
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [pendingSelection, setPendingSelection] =
    useState<{ start: number; end: number } | null>(null);

  const handleToolbarAction = (apply: (sel: TextSelection, end: number) => EditResult) => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart ?? value.length;
    const end = textarea.selectionEnd ?? value.length;
    const result = apply({ text: value, start }, end);
    // A toolbar action inserts characters of its own (** for bold, a link
    // skeleton), so it can cross the cap even though typing cannot. Drop the
    // action rather than silently truncating the user's text mid-token.
    if (result.text.length > maxLength) return;
    onChange(result.text);
    // `value` is a controlled prop owned by the caller, so the textarea only
    // reflects the new text once it flows back down as a re-render — an
    // effect keyed on `value` (rather than guessing the timing with
    // requestAnimationFrame) restores the selection right after that render
    // actually commits, however many ticks it takes.
    setPendingSelection({ start: result.selectionStart, end: result.selectionEnd });
  };

  useEffect(() => {
    if (!pendingSelection) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    textarea.setSelectionRange(pendingSelection.start, pendingSelection.end);
    setPendingSelection(null);
    // Re-runs on every new pending selection, keyed on `value` so it fires
    // after the textarea's DOM actually reflects the edited text.
  }, [value, pendingSelection]);

  return (
    <Box sx={{ width: "100%" }}>
      {label && <FormLabel htmlFor={inputId}>{label}</FormLabel>}
      <Box
        sx={{
          border: 1,
          borderColor: error ? "error.main" : "divider",
          borderRadius: 1,
          overflow: "hidden",
        }}
      >
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: 1,
            borderColor: "divider",
            bgcolor: "action.hover",
          }}
        >
          <Tabs
            value={tab}
            onChange={(_, next: MarkdownEditorTab) => setTab(next)}
            sx={{ minHeight: 36, "& .MuiTab-root": { minHeight: 36, py: 0.5 } }}
          >
            <Tab value="write" label="Write" />
            <Tab value="preview" label="Preview" />
          </Tabs>
          {tab === "write" && (
            <Stack direction="row" spacing={0.25} sx={{ pr: 0.75 }}>
              {TOOLBAR_ACTIONS.map(({ key, label: actionLabel, Icon, apply }) => (
                <Tooltip key={key} title={actionLabel}>
                  <span>
                    <IconButton
                      size="small"
                      disabled={disabled}
                      onClick={() => handleToolbarAction(apply)}
                      onMouseDown={(e) => e.preventDefault()}
                    >
                      <Icon size={15} />
                    </IconButton>
                  </span>
                </Tooltip>
              ))}
            </Stack>
          )}
        </Box>
        <Box sx={{ p: tab === "preview" ? 1.5 : 0, minHeight: 96 }}>
          {tab === "write" ? (
            <TextField
              id={inputId}
              inputRef={textareaRef}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={placeholder}
              multiline
              minRows={minRows}
              maxRows={maxRows}
              disabled={disabled}
              fullWidth
              variant="standard"
              slotProps={{
                input: { disableUnderline: true, sx: { px: 1.5, py: 1 } },
                htmlInput: { maxLength },
              }}
            />
          ) : value.trim() ? (
            <MarkdownView content={value} />
          ) : (
            <Typography variant="body2" color="text.disabled">
              Nothing to preview
            </Typography>
          )}
        </Box>
      </Box>
      {(helperText || error || !disabled) && (
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="flex-start"
          spacing={1}
        >
          <FormHelperText error={error} sx={{ mx: 1.75 }}>
            {helperText}
          </FormHelperText>
          {!disabled && (
            <FormHelperText
              sx={{ mx: 1.75, flexShrink: 0, fontVariantNumeric: "tabular-nums" }}
            >
              {value.length}/{maxLength}
            </FormHelperText>
          )}
        </Stack>
      )}
    </Box>
  );
};
