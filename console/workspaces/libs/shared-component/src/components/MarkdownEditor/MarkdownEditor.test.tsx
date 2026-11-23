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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { MarkdownEditor, type MarkdownEditorProps } from "./MarkdownEditor";

// Controlled host, as every caller owns the value.
function Host({
  initial = "",
  onChangeSpy,
  ...props
}: Partial<MarkdownEditorProps> & { initial?: string; onChangeSpy?: (v: string) => void }) {
  const [value, setValue] = useState(initial);
  return (
    <MarkdownEditor
      label="Description"
      value={value}
      onChange={(v) => {
        setValue(v);
        onChangeSpy?.(v);
      }}
      {...props}
    />
  );
}

const renderEditor = (props: Parameters<typeof Host>[0] = {}) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <Host {...props} />
    </ThemeProvider>,
  );

const textarea = () => screen.getByLabelText("Description") as HTMLTextAreaElement;
const toolbarButton = (label: string) =>
  screen.getByLabelText(label).querySelector("button") as HTMLButtonElement;

describe("MarkdownEditor", () => {
  it("edits text and shows the character count against the cap", () => {
    const onChangeSpy = vi.fn();
    renderEditor({ onChangeSpy, maxLength: 50, placeholder: "Describe it" });
    expect(screen.getByPlaceholderText("Describe it")).toBeInTheDocument();
    fireEvent.change(textarea(), { target: { value: "hello" } });
    expect(onChangeSpy).toHaveBeenCalledWith("hello");
    expect(screen.getByText("5/50")).toBeInTheDocument();
    expect(textarea()).toHaveAttribute("maxlength", "50");
  });

  it.each([
    ["Bold", "**world**"],
    ["Italic", "*world*"],
    ["Link", "[world](url)"],
  ])("%s wraps the selection and keeps it selected", (label, wrapped) => {
    renderEditor({ initial: "hello world" });
    textarea().setSelectionRange(6, 11);
    fireEvent.click(toolbarButton(label));
    expect(textarea()).toHaveValue(`hello ${wrapped}`);
    expect(document.activeElement).toBe(textarea());
  });

  it.each([
    ["Heading", "### "],
    ["Quote", "> "],
    ["Bulleted list", "- "],
  ])("%s prefixes the current line", (label, prefix) => {
    renderEditor({ initial: "first\nsecond" });
    textarea().setSelectionRange(8, 8);
    fireEvent.click(toolbarButton(label));
    expect(textarea()).toHaveValue(`first\n${prefix}second`);
    expect(textarea().selectionStart).toBe(8 + prefix.length);
  });

  it("drops a toolbar action that would exceed the cap", () => {
    const onChangeSpy = vi.fn();
    renderEditor({ initial: "abcd", maxLength: 6, onChangeSpy });
    textarea().setSelectionRange(0, 4);
    fireEvent.click(toolbarButton("Bold"));
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(textarea()).toHaveValue("abcd");
  });

  it("previews rendered markdown and hides the toolbar", () => {
    renderEditor({ initial: "**strong words**" });
    fireEvent.click(screen.getByRole("tab", { name: "Preview" }));
    expect(screen.getByText("strong words").tagName).toBe("STRONG");
    expect(screen.queryByLabelText("Bold")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Write" }));
    expect(textarea()).toHaveValue("**strong words**");
  });

  it("says when there is nothing to preview", () => {
    renderEditor({ initial: "   " });
    fireEvent.click(screen.getByRole("tab", { name: "Preview" }));
    expect(screen.getByText("Nothing to preview")).toBeInTheDocument();
  });

  it("shows helper text and hides the counter and toolbar actions when disabled", () => {
    renderEditor({ disabled: true, error: true, helperText: "Too vague" });
    expect(screen.getByText("Too vague")).toBeInTheDocument();
    expect(screen.queryByText(/^0\//)).not.toBeInTheDocument();
    expect(textarea()).toBeDisabled();
    expect(toolbarButton("Bold")).toBeDisabled();
  });
});
