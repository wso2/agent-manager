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

import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  LabelsEditor,
  MAX_LABELS_PER_RESOURCE,
  validateLabelKey,
  validateLabelValue,
} from "./LabelsEditor";

const FORMAT = "Use letters, digits, '.', '_' or '-', starting and ending with a letter or digit";

describe("validateLabelKey / validateLabelValue", () => {
  it.each([
    ["", "Key is required"],
    ["a".repeat(64), "Key must be at most 63 characters"],
    ["-bad", FORMAT],
    ["team.name_1", undefined],
  ])("key %j -> %j", (key, err) => expect(validateLabelKey(key)).toBe(err));

  it.each([
    ["", undefined],
    ["a".repeat(64), "Value must be at most 63 characters"],
    ["bad-", FORMAT],
    ["prod", undefined],
  ])("value %j -> %j", (value, err) => expect(validateLabelValue(value)).toBe(err));
});

type HarnessProps = {
  initial: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
} & Partial<React.ComponentProps<typeof LabelsEditor>>;

function Harness({ initial, onChange, ...rest }: HarnessProps) {
  const [value, setValue] = useState(initial);
  return (
    <LabelsEditor
      value={value}
      onChange={(v) => {
        setValue(v);
        onChange(v);
      }}
      {...rest}
    />
  );
}

describe("LabelsEditor", () => {
  it("shows existing labels and removes one", () => {
    const onChange = vi.fn();
    render(<Harness initial={{ env: "prod", tier: "" }} onChange={onChange} />);
    expect(screen.getByText("Labels (Optional)")).toBeInTheDocument();
    expect(screen.getByText("env: prod")).toBeInTheDocument();
    expect(screen.getByText("tier")).toBeInTheDocument();
    const chip = screen.getByText("env: prod").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(chip.querySelector("svg")!);
    expect(onChange).toHaveBeenCalledWith({ tier: "" });
  });

  it("adds a label after validating key, value and duplicates", () => {
    const onChange = vi.fn();
    render(<Harness initial={{ env: "prod" }} onChange={onChange} hideTitle description="Tag it" />);
    expect(screen.queryByText("Labels (Optional)")).not.toBeInTheDocument();
    expect(screen.getByText("Tag it")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add label" }));
    const add = screen.getByRole("button", { name: "Add" });
    expect(add).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText("e.g. env"), { target: { value: "env" } });
    expect(screen.getByText("A label with this key already exists")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("e.g. env"), { target: { value: "_x" } });
    expect(screen.getByText(FORMAT)).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("e.g. env"), { target: { value: "team" } });
    fireEvent.change(screen.getByPlaceholderText("e.g. production"), { target: { value: "bad value" } });
    expect(add).toBeDisabled();
    fireEvent.change(screen.getByPlaceholderText("e.g. production"), { target: { value: "core" } });
    fireEvent.click(add);
    expect(onChange).toHaveBeenCalledWith({ env: "prod", team: "core" });
    expect(screen.getByText("team: core")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("e.g. env")).not.toBeInTheDocument();
  });

  it("cancels the add form", () => {
    const onChange = vi.fn();
    render(<Harness initial={{}} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Add label" }));
    fireEvent.change(screen.getByPlaceholderText("e.g. env"), { target: { value: "x" } });
    fireEvent.click(screen.getByTitle("Cancel"));
    expect(screen.queryByPlaceholderText("e.g. env")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add label" }));
    expect((screen.getByPlaceholderText("e.g. env") as HTMLInputElement).value).toBe("");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("stops adding at the label limit", () => {
    const full = Object.fromEntries(
      Array.from({ length: MAX_LABELS_PER_RESOURCE }, (_, i) => [`k${i}`, "v"]),
    );
    render(<Harness initial={full} onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Add label" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add label" })).toHaveAttribute(
      "title",
      "At most 10 labels per resource",
    );
  });

  it("is read-only when disabled", () => {
    render(<Harness initial={{ env: "prod" }} onChange={vi.fn()} disabled />);
    expect(screen.queryByRole("button", { name: "Add label" })).not.toBeInTheDocument();
    const chip = screen.getByText("env: prod").closest(".MuiChip-root") as HTMLElement;
    expect(chip.querySelector("svg")).toBeNull();
  });
});
