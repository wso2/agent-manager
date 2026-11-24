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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import PolicyParameterEditor from "./PolicyParameterEditor";
import type {
  ParameterSchema,
  ParameterValues,
  PolicyDefinition,
} from "../../utils/policyParameterEditor";

const policy = (parameters: ParameterSchema, description = ""): PolicyDefinition => ({
  name: "my-policy",
  version: "1.0.0",
  description,
  parameters,
});

function renderEditor(
  definition: PolicyDefinition,
  props: Partial<React.ComponentProps<typeof PolicyParameterEditor>> = {},
) {
  const onSubmit = vi.fn<(v: ParameterValues) => void>();
  const onCancel = vi.fn();
  const utils = render(
    <ThemeProvider theme={createTheme()}>
      <PolicyParameterEditor
        policyDefinition={definition}
        onSubmit={onSubmit}
        onCancel={onCancel}
        {...props}
      />
    </ThemeProvider>,
  );
  return { ...utils, onSubmit, onCancel };
}

// Form.ElementWrapper renders the label as a sibling of the input inside one
// FormControl, so find the field through its label's container.
function fieldByLabel(label: string): HTMLElement {
  const container = screen.getByText(label).parentElement as HTMLElement;
  return container;
}
function inputByLabel(label: string, index = 0): HTMLInputElement {
  return fieldByLabel(label).querySelectorAll("input")[index] as HTMLInputElement;
}

// The editor's own Add/Save button is always the last one rendered (a
// key-value field has an "Add" button of its own).
const submitButton = () => screen.getAllByRole("button", { name: /^(Add|Save)$/ }).at(-1) as HTMLElement;

describe("PolicyParameterEditor", () => {
  it("shows the display name and truncates long descriptions", () => {
    const description = Array.from({ length: 7 }, (_, i) => `line ${i + 1}`).join("\n");
    renderEditor(policy({ type: "object", properties: {} }, description), {
      policyDisplayName: "Rate Limit",
    });

    expect(screen.getByText("Rate Limit")).toBeInTheDocument();
    expect(screen.queryByText(/line 6/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(screen.getByText(/line 7/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    expect(screen.queryByText(/line 7/)).not.toBeInTheDocument();
  });

  it("falls back to the policy name and omits the toggle for short descriptions", () => {
    renderEditor(policy({ type: "object", properties: {} }, "short"));
    expect(screen.getByText("my-policy")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  it("keeps Add disabled until required fields are filled, then submits coerced values", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        required: ["name"],
        properties: {
          name: { type: "string", description: "Header name" },
          retries: { type: "integer", default: 3 },
          ratio: { type: "number" },
          note: { type: "string" },
        },
      }),
    );

    expect(submitButton()).toBeDisabled();
    fireEvent.change(inputByLabel("* Name"), { target: { value: "X-Trace" } });
    fireEvent.change(inputByLabel("Ratio"), { target: { value: "0.5" } });
    expect(submitButton()).toBeEnabled();
    fireEvent.click(submitButton());

    // The optional, empty `note` is dropped rather than sent as "".
    expect(onSubmit).toHaveBeenCalledWith({ name: "X-Trace", retries: 3, ratio: 0.5 });
  });

  it("calls onCancel", () => {
    const { onCancel } = renderEditor(policy({ type: "object", properties: {} }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("prefills existing values in edit mode and reports dirtiness", () => {
    const onDirtyChange = vi.fn();
    const existingValues = { name: "old" };
    const { unmount, onSubmit } = renderEditor(
      policy({ type: "object", properties: { name: { type: "string" } } }),
      { isEditMode: true, existingValues, onDirtyChange },
    );

    expect(inputByLabel("Name")).toHaveValue("old");
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
    fireEvent.change(inputByLabel("Name"), { target: { value: "new" } });
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledWith({ name: "new" });

    unmount();
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
  });

  it("disables all actions when disabled", () => {
    renderEditor(policy({ type: "object", properties: { name: { type: "string" } } }), {
      disabled: true,
    });
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(submitButton()).toBeDisabled();
    expect(inputByLabel("Name")).toBeDisabled();
  });

  it("reports missing nested required fields and clears an error once edited", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        properties: {
          auth: {
            type: "object",
            required: ["token"],
            properties: { token: { type: "string" }, scheme: { type: "string" } },
          },
        },
      }),
    );

    // Partially filling the optional object makes its required children due.
    fireEvent.click(screen.getByText("auth"));
    fireEvent.change(inputByLabel("Scheme"), { target: { value: "Bearer" } });
    fireEvent.click(submitButton());

    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText("This field is required")).toBeInTheDocument();

    fireEvent.change(inputByLabel("* Token"), { target: { value: "abc" } });
    expect(screen.queryByText("This field is required")).not.toBeInTheDocument();
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ auth: { token: "abc", scheme: "Bearer" } });
  });

  it("enables Add once any anyOf branch is satisfied", () => {
    const schema: ParameterSchema = {
      type: "object",
      properties: { text: { type: "string" }, url: { type: "string" } },
      anyOf: [{ required: ["text"] }, { required: ["url"] }],
    };
    const { onSubmit } = renderEditor(policy(schema));
    expect(submitButton()).toBeDisabled();

    fireEvent.change(inputByLabel("Url"), { target: { value: "https://x" } });
    expect(submitButton()).toBeEnabled();
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ url: "https://x" });
  });

  it("asks for one of the branch fields when no nested branch matches", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        properties: {
          target: {
            type: "object",
            properties: {
              kind: { type: "string" },
              host: { type: "string" },
              path: { type: "string" },
            },
            anyOf: [
              { required: ["host"], properties: { kind: { const: "remote" } } },
              { required: ["path"], properties: { kind: { const: "local" } } },
            ],
          },
        },
      }),
    );
    fireEvent.click(screen.getByText("target"));
    fireEvent.change(inputByLabel("Kind"), { target: { value: "other" } });
    fireEvent.click(submitButton());
    expect(onSubmit).not.toHaveBeenCalled();
    // kind, host and path are all flagged.
    expect(screen.getAllByText("At least one of these fields is required")).toHaveLength(3);
  });

  it("flags only the active nested branch's missing fields", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        properties: {
          target: {
            type: "object",
            properties: {
              kind: { type: "string" },
              host: { type: "string" },
              path: { type: "string" },
            },
            anyOf: [
              { required: ["host"], properties: { kind: { const: "remote" } } },
              { required: ["path"], properties: { kind: { const: "local" } } },
            ],
          },
        },
      }),
    );
    fireEvent.click(screen.getByText("target"));
    fireEvent.change(inputByLabel("Kind"), { target: { value: "local" } });
    fireEvent.click(submitButton());
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getAllByText("This field is required")).toHaveLength(1);
    fireEvent.change(inputByLabel("Path"), { target: { value: "/srv" } });
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ target: { kind: "local", path: "/srv" } });
  });

  it("flags the active discriminated branch's missing fields", () => {
    const schema: ParameterSchema = {
      type: "object",
      properties: {
        enabled: { type: "boolean" },
        endpoint: { type: "string" },
      },
      anyOf: [
        { required: ["enabled"], properties: { enabled: { const: false } } },
        { required: ["enabled", "endpoint"], properties: { enabled: { const: true } } },
      ],
    };
    const { onSubmit } = renderEditor(policy(schema));

    // Starts on the disabled branch, which is satisfied.
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenLastCalledWith({ enabled: false });

    fireEvent.click(within(fieldByLabel("Enabled")).getByRole("switch"));
    expect(submitButton()).toBeDisabled();
  });

  it("renders enum, boolean, tag-list and key-value fields and submits their values", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        properties: {
          mode: { type: "string", enum: ["allow", "deny"], description: "Pick a mode" },
          strict: { type: "boolean", description: "Be strict" },
          tags: { type: "array", items: { type: "string" } },
          ports: { type: "array", items: { type: "integer" } },
          headers: { type: "object", additionalProperties: { type: "string" } },
        },
      }),
    );

    // Enum select
    fireEvent.mouseDown(within(fieldByLabel("Mode")).getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "deny" }));

    // Boolean switch
    expect(screen.getByText("Be strict")).toBeInTheDocument();
    fireEvent.click(within(fieldByLabel("Strict")).getByRole("switch"));

    // Tag input: Enter and comma add, duplicates ignored, Backspace removes last.
    const tagInput = inputByLabel("Tags");
    fireEvent.change(tagInput, { target: { value: "a" } });
    fireEvent.keyDown(tagInput, { key: "Enter" });
    fireEvent.change(tagInput, { target: { value: "b" } });
    fireEvent.keyDown(tagInput, { key: "," });
    fireEvent.change(tagInput, { target: { value: "a" } });
    fireEvent.keyDown(tagInput, { key: "Enter" });
    fireEvent.change(tagInput, { target: { value: "c" } });
    fireEvent.keyDown(tagInput, { key: "Enter" });
    fireEvent.keyDown(tagInput, { key: "Backspace" });
    expect(within(fieldByLabel("Tags")).getByText("a")).toBeInTheDocument();
    expect(within(fieldByLabel("Tags")).queryByText("c")).not.toBeInTheDocument();

    const portInput = inputByLabel("Ports");
    fireEvent.change(portInput, { target: { value: "8080" } });
    fireEvent.keyDown(portInput, { key: "Enter" });

    // Key-value: add two entries, edit one, delete the other.
    const kv = fieldByLabel("Headers");
    const keyInput = within(kv).getByPlaceholderText("Key");
    const valueInput = within(kv).getByPlaceholderText("Value");
    const addKv = within(kv).getByRole("button", { name: "Add" });
    expect(addKv).toBeDisabled();
    fireEvent.change(keyInput, { target: { value: "X-One" } });
    fireEvent.change(valueInput, { target: { value: "1" } });
    fireEvent.click(addKv);
    fireEvent.change(keyInput, { target: { value: "X-Two" } });
    fireEvent.change(valueInput, { target: { value: "2" } });
    fireEvent.keyDown(valueInput, { key: "Enter" });
    expect(within(kv).getByDisplayValue("X-Two")).toBeInTheDocument();
    fireEvent.change(within(kv).getByDisplayValue("1"), { target: { value: "one" } });
    const rowTwo = within(kv).getByDisplayValue("X-Two").closest(".MuiStack-root") as HTMLElement;
    fireEvent.click(within(rowTwo).getByRole("button"));

    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({
      mode: "deny",
      strict: true,
      tags: ["a", "b"],
      ports: [8080],
      headers: { "X-One": "one" },
    });
  });

  it("removes a tag through its chip", () => {
    const { onSubmit } = renderEditor(
      policy({ type: "object", properties: { tags: { type: "array", items: { type: "string" } } } }),
      { existingValues: { tags: ["keep", "drop"] } },
    );
    const chip = within(fieldByLabel("Tags")).getByText("drop").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(chip.querySelector("svg") as SVGElement);
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ tags: ["keep"] });
  });

  it("clears a number field to an empty value and ignores invalid input", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        required: ["limit"],
        properties: { limit: { type: "integer", minimum: 1 } },
      }),
    );
    const input = inputByLabel("* Limit");
    expect(input).toHaveValue(1);
    fireEvent.change(input, { target: { value: "" } });
    expect(submitButton()).toBeDisabled();
    fireEvent.change(input, { target: { value: "25" } });
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ limit: 25 });
  });

  it("adds and deletes complex array items", () => {
    const { onSubmit } = renderEditor(
      policy({
        type: "object",
        properties: {
          rules: {
            type: "array",
            description: "Routing rules",
            items: {
              type: "object",
              required: ["path"],
              properties: { path: { type: "string" } },
            },
          },
        },
      }),
    );

    fireEvent.click(screen.getByText("rules"));
    expect(screen.getByText("Routing rules")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add item" }));
    fireEvent.click(screen.getByRole("button", { name: "Add item" }));
    expect(screen.getByText("Item 2")).toBeInTheDocument();

    // Submitting with empty required item fields flags them.
    fireEvent.click(submitButton());
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getAllByText("This field is required").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Delete Item 2" }));
    expect(screen.queryByText("Item 2")).not.toBeInTheDocument();
    fireEvent.change(inputByLabel("* Path"), { target: { value: "/a" } });
    fireEvent.click(submitButton());
    expect(onSubmit).toHaveBeenCalledWith({ rules: [{ path: "/a" }] });
  });

  it("splits fields into main and advanced sections by the advanced-param flag", () => {
    renderEditor(
      policy({
        type: "object",
        required: ["required"],
        properties: {
          main: { type: "string", "x-wso2-policy-advanced-param": false },
          tuning: { type: "string", "x-wso2-policy-advanced-param": true },
          required: { type: "string" },
          leftover: { type: "string" },
          group: {
            type: "object",
            required: ["inner"],
            properties: {
              inner: { type: "string" },
              extra: { type: "string", "x-wso2-policy-advanced-param": true },
            },
          },
        },
      }),
    );

    const [nestedAdvanced, rootAdvanced] = screen
      .getAllByText("Advanced Settings")
      .map((el) => el.closest(".MuiAccordion-root") as HTMLElement);
    const advanced = rootAdvanced;
    expect(within(advanced).getByText("Tuning")).toBeInTheDocument();
    expect(within(advanced).getByText("Leftover")).toBeInTheDocument();
    expect(within(advanced).queryByText("Main")).not.toBeInTheDocument();
    expect(within(advanced).queryByText("* Required")).not.toBeInTheDocument();

    // Nested objects get their own advanced section too.
    expect(within(nestedAdvanced).getByText("Extra")).toBeInTheDocument();
    expect(within(nestedAdvanced).queryByText("* Inner")).not.toBeInTheDocument();
  });
});
