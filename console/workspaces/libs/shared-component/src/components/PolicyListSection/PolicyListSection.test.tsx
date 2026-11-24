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

import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type { GuardrailDefinition } from "@agent-management-platform/api-client";
import type { PolicySelectorDrawerProps } from "../PolicySelectorDrawer/PolicySelectorDrawer";
import { PolicyListSection, type PolicySelection } from "./PolicyListSection";

// The drawer has its own tests; stub it so these can drive its callbacks and
// read the props the section hands it.
let drawerProps: PolicySelectorDrawerProps;
vi.mock("../PolicySelectorDrawer/PolicySelectorDrawer", () => ({
  PolicySelectorDrawer: (props: PolicySelectorDrawerProps) => {
    drawerProps = props;
    return props.open ? <div data-testid="drawer">{props.title}</div> : null;
  },
}));

const policies: PolicySelection[] = [
  { name: "pii", version: "1.0.0", displayName: "PII Masking", settings: { a: 1 } },
  { name: "word-count", version: "2.0.0" },
  { name: "regex", version: "1.0.0", displayName: "Regex" },
];

const picked = { name: "new-one", version: "3.0.0", displayName: "New One" } as GuardrailDefinition;

function renderSection(props: Partial<React.ComponentProps<typeof PolicyListSection>> = {}) {
  const handlers = { onAdd: vi.fn(), onEdit: vi.fn(), onRemove: vi.fn(), onReorder: vi.fn() };
  render(
    <ThemeProvider theme={createTheme()}>
      <PolicyListSection policies={policies} {...handlers} {...props} />
    </ThemeProvider>,
  );
  return handlers;
}

const card = (label: string) => screen.getByText(label).closest(".MuiCard-root") as HTMLElement;

beforeEach(() => {
  drawerProps = undefined as unknown as PolicySelectorDrawerProps;
});

describe("PolicyListSection", () => {
  it("shows an empty state named after the section", () => {
    renderSection({ policies: [], title: "Guardrails" });
    expect(screen.getByText("No guardrails added yet")).toBeInTheDocument();
  });

  it("lists policies by display name, falling back to the name, with positions", () => {
    renderSection();
    expect(screen.getByText("PII Masking")).toBeInTheDocument();
    expect(screen.getByText("word-count")).toBeInTheDocument();
    expect(screen.getByLabelText("Position 3. Drag to reorder.")).toBeInTheDocument();
  });

  it("adds a policy through the drawer, disabling ones already added", () => {
    const { onAdd } = renderSection({ addButtonLabel: "Add guardrail" });
    expect(drawerProps.open).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Add guardrail" }));
    expect(screen.getByTestId("drawer")).toHaveTextContent("Add Policy");
    expect(drawerProps.disabledPolicyKeys).toEqual(["pii@1.0.0", "word-count@2.0.0", "regex@1.0.0"]);
    expect(drawerProps.editPolicyKey).toBeUndefined();

    act(() => drawerProps.onSubmit(picked, { limit: 5 }));
    expect(onAdd).toHaveBeenCalledWith({
      name: "new-one",
      version: "3.0.0",
      displayName: "New One",
      settings: { limit: 5 },
    });
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
  });

  it("edits a policy when its row is clicked", () => {
    const { onEdit, onAdd } = renderSection();
    fireEvent.click(card("PII Masking"));

    expect(screen.getByTestId("drawer")).toHaveTextContent("Edit Policy");
    expect(drawerProps.editPolicyKey).toBe("pii@1.0.0");
    expect(drawerProps.existingSettings).toEqual({ a: 1 });
    expect(drawerProps.disabledPolicyKeys).toEqual([]);
    expect(drawerProps.subtitle).toBe("Update the policy configuration.");

    act(() => drawerProps.onSubmit({ ...picked, name: "pii", version: "1.0.0" }, { a: 2 }));
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ name: "pii", settings: { a: 2 } }));
    expect(onAdd).not.toHaveBeenCalled();
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
  });

  it("closes the drawer and resets edit state", () => {
    renderSection();
    fireEvent.click(card("Regex"));
    act(() => drawerProps.onClose());
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
    expect(drawerProps.editPolicyKey).toBeUndefined();
  });

  it("removes a policy without opening the editor", () => {
    const { onRemove } = renderSection();
    fireEvent.click(screen.getByRole("button", { name: "Remove word-count" }));
    expect(onRemove).toHaveBeenCalledWith("word-count", "2.0.0");
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
  });

  it("reorders by drag and drop", () => {
    const { onReorder } = renderSection();
    const dataTransfer = { setData: vi.fn(), effectAllowed: "", dropEffect: "" };

    fireEvent.dragStart(card("PII Masking"), { dataTransfer });
    fireEvent.dragOver(card("Regex"), { dataTransfer });
    fireEvent.drop(card("Regex"), { dataTransfer });

    expect(onReorder).toHaveBeenCalledWith([policies[1], policies[2], policies[0]]);
  });

  it("ignores a drop on the dragged row itself and a drop without a drag", () => {
    const { onReorder } = renderSection();
    const dataTransfer = { setData: vi.fn(), effectAllowed: "", dropEffect: "" };

    fireEvent.drop(card("Regex"), { dataTransfer });
    fireEvent.dragStart(card("Regex"), { dataTransfer });
    fireEvent.drop(card("Regex"), { dataTransfer });
    fireEvent.dragStart(card("Regex"), { dataTransfer });
    fireEvent.dragEnd(card("Regex"));
    fireEvent.drop(card("PII Masking"), { dataTransfer });

    expect(onReorder).not.toHaveBeenCalled();
  });

  it("has no position handles when reordering is not supported", () => {
    renderSection({ onReorder: undefined });
    expect(screen.queryByLabelText(/Drag to reorder/)).not.toBeInTheDocument();
  });

  it("is view-only when readOnly", () => {
    const { onReorder } = renderSection({ readOnly: true });
    expect(screen.queryByRole("button", { name: "Add policy" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Remove/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Drag to reorder/)).not.toBeInTheDocument();

    fireEvent.click(card("PII Masking"));
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();

    const dataTransfer = { setData: vi.fn(), effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(card("PII Masking"), { dataTransfer });
    fireEvent.drop(card("Regex"), { dataTransfer });
    expect(onReorder).not.toHaveBeenCalled();
  });
});
