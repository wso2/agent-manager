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

import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

// Dialog transitions make a few of these slow on a busy machine.
vi.setConfig({ testTimeout: 60000 });

// swagger-ui-react does not render under jsdom; record what it was handed.
vi.mock("swagger-ui-react", () => ({
  default: (props: { spec?: { paths?: Record<string, unknown> } }) => (
    <div data-testid="swagger">{JSON.stringify(props.spec?.paths ?? null)}</div>
  ),
}));

import {
  AccessControlPanel,
  type AccessControlItem,
  type AccessControlMode,
  type AccessControlPanelProps,
} from "./AccessControlPanel";

const items: AccessControlItem[] = [
  { key: "get-pets", method: "GET", path: "/pets", summary: "List pets" },
  { key: "post-pets", method: "POST", path: "/pets", summary: "Create pet" },
  { key: "del-pet", method: "DELETE", path: "/pets/{id}" },
  { key: "tool", method: "TOOL", path: "search_docs" },
];

// Parent owns mode and exception keys, as in the real pages.
function Harness({
  initialMode = "allow",
  initialExceptions = [],
  onKeys,
  onModeChange,
  ...rest
}: Partial<AccessControlPanelProps> & {
  initialMode?: AccessControlMode;
  initialExceptions?: string[];
  onKeys?: (keys: string[]) => void;
}) {
  const [mode, setMode] = useState<AccessControlMode>(initialMode);
  const [keys, setKeys] = useState<string[]>(initialExceptions);
  return (
    <AccessControlPanel
      items={items}
      isDirty={false}
      onSave={vi.fn()}
      onDiscard={vi.fn()}
      {...rest}
      mode={mode}
      onModeChange={(m) => {
        setMode(m);
        onModeChange?.(m);
      }}
      exceptionKeys={keys}
      onExceptionKeysChange={(k) => {
        setKeys(k);
        onKeys?.(k);
      }}
    />
  );
}

const sections = () => {
  const [left, right] = screen.getAllByPlaceholderText("Search resources").map(
    (input) => input.closest(".MuiFormControl-root")!.parentElement as HTMLElement,
  );
  return { left, right };
};
const iconButton = (title: string) =>
  screen.getByLabelText(title).querySelector("button") ??
  (screen.getByLabelText(title) as HTMLElement);

// The toggle's accessible name also swallows its help tooltip, so find it by value.
const modeToggle = (value: AccessControlMode) =>
  document.querySelector(`button[value="${value}"]`) as HTMLElement;

const columnHeadings = () =>
  screen.getAllByText(/^(Allowed|Denied) Resources$/).map((el) => el.textContent);

describe("AccessControlPanel", () => {
  it("renders skeletons while loading", () => {
    const { container } = render(<Harness isLoading />);
    expect(container.querySelectorAll(".MuiSkeleton-root").length).toBe(4);
    expect(screen.queryByText("Mode")).not.toBeInTheDocument();
  });

  it("lists every item as allowed with an empty exceptions column in allow mode", () => {
    render(<Harness />);
    expect(columnHeadings()).toEqual(["Allowed Resources", "Denied Resources"]);
    const { left, right } = sections();
    expect(within(left).getAllByText("/pets")).toHaveLength(2);
    expect(within(left).getByText("search_docs")).toBeInTheDocument();
    expect(within(right).getByText("No selected resources")).toBeInTheDocument();
  });

  it("moves selected items to exceptions and back", () => {
    const onKeys = vi.fn();
    render(<Harness onKeys={onKeys} />);
    expect(iconButton("Move selected to exceptions")).toBeDisabled();
    fireEvent.click(screen.getByText("List pets"));
    fireEvent.click(screen.getByText("search_docs"));
    fireEvent.click(screen.getByText("search_docs")); // toggled off again
    fireEvent.click(iconButton("Move selected to exceptions"));
    expect(onKeys).toHaveBeenLastCalledWith(["get-pets"]);
    const { right } = sections();
    expect(within(right).getByText("List pets")).toBeInTheDocument();

    fireEvent.click(within(right).getByText("List pets"));
    fireEvent.click(iconButton("Move selected to available"));
    expect(onKeys).toHaveBeenLastCalledWith([]);
  });

  it("moves everything across with the double-arrow buttons", () => {
    const onKeys = vi.fn();
    render(<Harness onKeys={onKeys} />);
    fireEvent.click(iconButton("Move all to exceptions"));
    expect(onKeys).toHaveBeenLastCalledWith(items.map((i) => i.key));
    expect(sections().left.textContent).toContain("No available resources");
    fireEvent.click(iconButton("Move all to available"));
    expect(onKeys).toHaveBeenLastCalledWith([]);
  });

  it("filters each column by method, path and summary", () => {
    render(<Harness initialExceptions={["del-pet", "missing-key"]} />);
    const [leftSearch, rightSearch] = screen.getAllByPlaceholderText("Search resources");
    fireEvent.change(leftSearch, { target: { value: "create" } });
    const { left, right } = sections();
    expect(within(left).getByText("Create pet")).toBeInTheDocument();
    expect(within(left).queryByText("List pets")).not.toBeInTheDocument();
    // The unknown exception key is dropped, not rendered.
    expect(within(right).getByText("/pets/{id}")).toBeInTheDocument();
    fireEvent.change(rightSearch, { target: { value: "nothing" } });
    expect(within(right).getByText("No selected resources")).toBeInTheDocument();
  });

  it("asks before switching mode and applies only on confirm", () => {
    const onModeChange = vi.fn();
    render(<Harness onModeChange={onModeChange} />);
    fireEvent.click(modeToggle("deny"));
    expect(screen.getByText("Change resource mode from Allow all to Deny all?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onModeChange).not.toHaveBeenCalled();
    expect(columnHeadings()).toEqual(["Allowed Resources", "Denied Resources"]);

    fireEvent.click(modeToggle("deny"));
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(onModeChange).toHaveBeenCalledWith("deny");
    // The left column now lists what is denied.
    expect(columnHeadings()).toEqual(["Denied Resources", "Allowed Resources"]);
    // Picking the active mode again is a no-op.
    fireEvent.click(modeToggle("deny"));
    expect(onModeChange).toHaveBeenCalledTimes(1);
  });

  it("enables save and discard only when dirty and reports saving", () => {
    const onSave = vi.fn();
    const onDiscard = vi.fn();
    const { rerender } = render(<Harness onSave={onSave} onDiscard={onDiscard} />);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    rerender(<Harness onSave={onSave} onDiscard={onDiscard} isDirty />);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(onSave).toHaveBeenCalled();
    expect(onDiscard).toHaveBeenCalled();
    rerender(<Harness isDirty isSaving />);
    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
  });

  it("shows a dismissable status alert", () => {
    const onClearStatus = vi.fn();
    render(
      <Harness status={{ severity: "error", message: "Save failed" }} onClearStatus={onClearStatus} />,
    );
    expect(screen.getByText("Save failed")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClearStatus).toHaveBeenCalled();
  });

  it("hands an imported spec file to the parent", async () => {
    const onImportSpec = vi.fn();
    const { container } = render(<Harness showImportButton onImportSpec={onImportSpec} />);
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const click = vi.spyOn(input, "click");
    fireEvent.click(screen.getByRole("button", { name: /Import from specification/ }));
    expect(click).toHaveBeenCalled();
    const file = new File(["{}"], "spec.json", { type: "application/json" });
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() => expect(onImportSpec).toHaveBeenCalledWith(file));
  });

  it("hides every editing control in read-only mode", () => {
    render(<Harness readOnly showImportButton />);
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Import from specification/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Move all to exceptions")).not.toBeInTheDocument();
  });

  it("expands a row to show its operation in the spec viewer", async () => {
    render(<Harness />);
    const [firstExpand] = screen.getAllByRole("button", { name: "Expand details" });
    fireEvent.click(firstExpand);
    const viewer = await screen.findByTestId("swagger");
    expect(JSON.parse(viewer.textContent!)).toEqual({
      "/pets": { get: { summary: "List pets", responses: { default: { description: "No response details available." } } } },
    });
    fireEvent.click(screen.getByRole("button", { name: "Collapse details" }));
    await waitFor(() => expect(screen.queryByTestId("swagger")).not.toBeInTheDocument(), {
      timeout: 10000,
    });
  });
});
