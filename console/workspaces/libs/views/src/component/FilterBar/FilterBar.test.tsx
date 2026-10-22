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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ReactElement } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { FilterBar, FilterChip, FilterPanel, FilterToggle } from "./index";

const renderThemed = (ui: ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{ui}</ThemeProvider>);

const KIND_OPTIONS = [
  { value: "platform", label: "Platform" },
  { value: "external", label: "External" },
  { value: "proxied", label: "Proxied" },
];

describe("FilterToggle", () => {
  const renderToggle = (value?: string) => {
    const onChange = vi.fn();
    renderThemed(
      <FilterToggle label="Kind" value={value} options={KIND_OPTIONS} onChange={onChange} />,
    );
    const group = screen.getByRole("group", { name: "Kind" });
    return { onChange, group };
  };

  it("renders All pressed when no value is set", () => {
    const { group } = renderToggle();
    const names = within(group).getAllByRole("button").map((b) => b.textContent);
    expect(names).toEqual(["All", "Platform", "External", "Proxied"]);
    expect(within(group).getByRole("button", { name: "All" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(within(group).getByRole("button", { name: "External" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("emits the option value", () => {
    const { group, onChange } = renderToggle();
    fireEvent.click(within(group).getByRole("button", { name: "External" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("external");
  });

  it("emits undefined for All", () => {
    const { group, onChange } = renderToggle("external");
    fireEvent.click(within(group).getByRole("button", { name: "All" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith(undefined);
  });

  it("ignores a click on the pressed button", () => {
    const { group, onChange } = renderToggle("external");
    fireEvent.click(within(group).getByRole("button", { name: "External" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe("FilterChip", () => {
  const renderChip = (label?: string) => {
    const onOpen = vi.fn();
    const onRemove = vi.fn();
    renderThemed(
      <FilterChip
        name="Card status"
        value="Failed"
        label={label}
        onOpen={onOpen}
        onRemove={onRemove}
      />,
    );
    return { onOpen, onRemove };
  };

  it("shows name and value and names its remove button", () => {
    renderChip("Card status: Failed");
    const body = screen.getByTitle("Card status: Failed");
    expect(within(body).getByText("Card status")).toBeInTheDocument();
    expect(within(body).getByText("Failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Card status: Failed" })).toBeInTheDocument();
  });

  it("calls onOpen and onRemove", () => {
    const { onOpen, onRemove } = renderChip("Card status: Failed");
    fireEvent.click(screen.getByTitle("Card status: Failed"));
    expect(onOpen).toHaveBeenCalledOnce();
    expect(onRemove).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Remove Card status: Failed" }));
    expect(onRemove).toHaveBeenCalledOnce();
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it("defaults the label to name and value", () => {
    renderChip();
    expect(screen.getByTitle("Card status Failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Card status Failed" })).toBeInTheDocument();
  });
});

describe("FilterPanel", () => {
  let anchor: HTMLButtonElement;

  beforeEach(() => {
    anchor = document.createElement("button");
    document.body.appendChild(anchor);
  });

  afterEach(() => {
    anchor.remove();
  });

  const renderPanel = (applyDisabled?: boolean) => {
    const handlers = { onReset: vi.fn(), onApply: vi.fn(), onClose: vi.fn() };
    renderThemed(
      <FilterPanel
        anchorEl={anchor}
        id="x"
        title="Advanced Filters"
        width={640}
        applyDisabled={applyDisabled}
        {...handlers}
      >
        <span>fields</span>
      </FilterPanel>,
    );
    return handlers;
  };

  it("is a focused aside with heading, close, Reset and Filter", () => {
    const { onReset, onApply, onClose } = renderPanel();
    const panel = screen.getByRole("complementary", { name: "Advanced Filters" });
    expect(panel).toHaveAttribute("id", "x");
    expect(panel).toHaveFocus();
    expect(within(panel).getByText("fields")).toBeInTheDocument();
    expect(
      within(panel).getByRole("heading", { level: 2, name: "Advanced Filters" }),
    ).toBeInTheDocument();

    fireEvent.click(within(panel).getByRole("button", { name: "Close advanced filters" }));
    expect(onClose).toHaveBeenCalledOnce();
    fireEvent.click(within(panel).getByRole("button", { name: "Reset" }));
    expect(onReset).toHaveBeenCalledOnce();
    fireEvent.click(within(panel).getByRole("button", { name: "Filter" }));
    expect(onApply).toHaveBeenCalledOnce();
  });

  it("disables Filter when applyDisabled", () => {
    const { onApply } = renderPanel(true);
    const filter = screen.getByRole("button", { name: "Filter" });
    expect(filter).toBeDisabled();
    fireEvent.click(filter);
    expect(onApply).not.toHaveBeenCalled();
  });

  it("closes on Escape anywhere", () => {
    const { onClose } = renderPanel();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledOnce();
  });
});

describe("FilterBar", () => {
  it("renders actions before children", () => {
    renderThemed(
      <FilterBar actions={<span>act</span>}>
        <span>child</span>
      </FilterBar>,
    );
    const actions = screen.getByText("act");
    const child = screen.getByText("child");
    expect(
      actions.compareDocumentPosition(child) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
