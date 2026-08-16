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
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { PermissionTree, type PermissionTreeItem } from "./PermissionTree";

const renderWithTheme = (component: React.ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{component}</ThemeProvider>);

const CATALOG: PermissionTreeItem[] = [
  { id: "amp:monitor", path: ["amp", "monitor"] },
  { id: "amp:role", path: ["amp", "role"] },
  { id: "amp:catalog", path: ["amp", "catalog"] },
  { id: "amp:catalog:read", path: ["amp", "catalog", "read"] },
];

// Controlled wrapper so onChange results actually flow back into selectedIds,
// mirroring how the two consumer role-edit pages own selection state.
const ControlledTree: React.FC<{
  items: PermissionTreeItem[];
  initialSelected: string[];
  readOnly?: boolean;
  onChangeSpy?: (ids: string[]) => void;
}> = ({ items, initialSelected, readOnly, onChangeSpy }) => {
  const [selected, setSelected] = useState(initialSelected);
  return (
    <PermissionTree
      items={items}
      selectedIds={selected}
      readOnly={readOnly}
      onChange={(ids) => {
        setSelected(ids);
        onChangeSpy?.(ids);
      }}
    />
  );
};

const getCheckbox = (name: string) =>
  screen.getByRole("checkbox", { name: new RegExp(name) });

describe("PermissionTree", () => {
  it("renders group nodes derived from colon-namespaced ids without altering leaf ids", () => {
    renderWithTheme(<ControlledTree items={CATALOG} initialSelected={[]} />);
    expect(screen.getByText("Monitor")).toBeInTheDocument();
    expect(screen.getByText("Catalog")).toBeInTheDocument();
    expect(screen.getByText("amp:monitor")).toBeInTheDocument();
    // "read" is collapsed under "Catalog" until expanded.
    expect(screen.queryByText("amp:catalog:read")).not.toBeInTheDocument();
  });

  it("shows a leaf as checked when its exact id is selected", () => {
    renderWithTheme(
      <ControlledTree items={CATALOG} initialSelected={["amp:monitor"]} />,
    );
    expect(getCheckbox("Monitor")).toBeChecked();
    expect(getCheckbox("Role")).not.toBeChecked();
  });

  it("marks a parent indeterminate when only a descendant is selected, matching the own-scope-unset case", () => {
    renderWithTheme(
      <ControlledTree
        items={CATALOG}
        initialSelected={["amp:catalog:read"]}
      />,
    );
    const catalogCheckbox = getCheckbox("Catalog") as HTMLInputElement;
    expect(catalogCheckbox.checked).toBe(false);
    // MUI's Checkbox deliberately doesn't set the native `indeterminate` DOM
    // property (cross-browser inconsistency); it exposes `data-indeterminate` instead.
    expect(catalogCheckbox.getAttribute("data-indeterminate")).toBe("true");
  });

  it("shows a parent fully checked once both its own scope and all descendants are selected", () => {
    renderWithTheme(
      <ControlledTree
        items={CATALOG}
        initialSelected={["amp:catalog", "amp:catalog:read"]}
      />,
    );
    const catalogCheckbox = getCheckbox("Catalog") as HTMLInputElement;
    expect(catalogCheckbox.checked).toBe(true);
    expect(catalogCheckbox.indeterminate).toBe(false);
  });

  it("expands a group via the chevron to reveal its children", () => {
    renderWithTheme(<ControlledTree items={CATALOG} initialSelected={[]} />);
    expect(screen.queryByText("amp:catalog:read")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Expand/i }));
    expect(screen.getByText("amp:catalog:read")).toBeInTheDocument();
  });

  it("cascades a parent checkbox click to select its own scope plus all descendants", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledTree
        items={CATALOG}
        initialSelected={[]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Catalog"));
    expect(onChangeSpy).toHaveBeenCalledWith(
      expect.arrayContaining(["amp:catalog", "amp:catalog:read"]),
    );
    expect(getCheckbox("Catalog")).toBeChecked();
  });

  it("cascades unchecking a fully-selected parent to clear its whole subtree", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledTree
        items={CATALOG}
        initialSelected={["amp:catalog", "amp:catalog:read", "amp:monitor"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Catalog"));
    expect(onChangeSpy).toHaveBeenCalledWith(["amp:monitor"]);
  });

  it("disables all checkboxes in read-only mode", () => {
    renderWithTheme(
      <ControlledTree items={CATALOG} initialSelected={["amp:monitor"]} readOnly />,
    );
    expect(getCheckbox("Monitor")).toBeDisabled();
    expect(getCheckbox("Role")).toBeDisabled();
  });

  it("shows the empty message when the catalog has no items", () => {
    renderWithTheme(
      <ControlledTree
        items={[]}
        initialSelected={[]}
      />,
    );
    expect(screen.getByText("No permissions available.")).toBeInTheDocument();
  });

  it("keeps ids outside the catalog (orphaned selections) untouched by unrelated toggles", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledTree
        items={CATALOG}
        initialSelected={["amp:catalog:write:legacy"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Monitor"));
    const nextSelected = onChangeSpy.mock.calls[0][0] as string[];
    expect(nextSelected).toContain("amp:catalog:write:legacy");
    expect(nextSelected).toContain("amp:monitor");
  });
});
