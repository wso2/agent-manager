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

import { describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import type { GuardrailsCatalogResponse } from "@agent-management-platform/api-client";
import {
  policy,
  renderWithProviders,
} from "../PolicySelectorDrawer/__test__/helpers";

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/api-client")>()),
  useLLMPoliciesCatalog: () => ({
    data: undefined,
    isLoading: false,
    error: null,
  }),
  useTrack: () => ({ track: vi.fn() }),
}));
vi.mock(
  "../ConfirmationDialog/ConfirmationDialogProvider",
  () => ({ useConfirmationDialog: () => ({ addConfirmation: vi.fn() }) }),
);

import { PolicyListSection, type PolicySelection } from "./PolicyListSection";

const catalog = (
  data: GuardrailsCatalogResponse["data"],
  overrides: Partial<GuardrailsCatalogResponse> = {},
): GuardrailsCatalogResponse => ({
  count: data.length,
  data,
  hubAvailable: true,
  availabilityScope: "deployed",
  ...overrides,
});

const renderList = (
  props: Partial<React.ComponentProps<typeof PolicyListSection>> = {},
  policies: PolicySelection[] = [],
) => {
  const onAdd = vi.fn();
  const onEdit = vi.fn();
  const onRemove = vi.fn();
  renderWithProviders(
    <PolicyListSection
      policies={policies}
      onAdd={onAdd}
      onEdit={onEdit}
      onRemove={onRemove}
      filterPolicies={(items) => items}
      {...props}
    />,
  );
  return { onAdd, onEdit, onRemove };
};

const openPicker = () =>
  fireEvent.click(screen.getByRole("button", { name: /add policy/i }));

describe("PolicyListSection catalog picker", () => {
  it("disables not-installed catalog rows and shows the missing gateways", () => {
    renderList({
      catalogData: catalog([
        policy(),
        policy({
          name: "word-filter",
          displayName: "Word Filter",
          status: "not_installed",
          parameters: undefined,
          missingOn: [{ id: "g2", name: "gw-eu" }],
        }),
      ]),
    });
    openPicker();

    const row = screen.getByRole("button", { name: /Word Filter/ });
    expect(row).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText("Missing on gw-eu")).toBeInTheDocument();

    fireEvent.click(row);
    expect(screen.queryByRole("button", { name: "Back" })).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /PII Masking/ }),
    ).not.toHaveAttribute("aria-disabled", "true");
  });

  it("falls back to a generic note when no gateway is listed", () => {
    renderList({
      catalogData: catalog([
        policy({ status: "not_installed", parameters: undefined }),
      ]),
    });
    openPicker();
    expect(screen.getByText("Not installed on this gateway")).toBeInTheDocument();
  });

  it("lists installed rows before not-installed ones", () => {
    renderList({
      catalogData: catalog([
        policy({ name: "a-missing", displayName: "A Missing", status: "not_installed", parameters: undefined }),
        policy({ name: "z-installed", displayName: "Z Installed" }),
      ]),
    });
    openPicker();
    const rows = screen
      .getAllByRole("button")
      .filter((b) => /A Missing|Z Installed/.test(b.textContent ?? ""));
    expect(rows[0]).toHaveTextContent("Z Installed");
  });

  it("shows Custom chip", () => {
    renderList({
      catalogData: catalog([policy({ source: "custom", provider: "custom" })]),
    });
    openPicker();
    expect(screen.getByText("Custom")).toBeInTheDocument();
  });

  it("shows org-managed scope note", () => {
    renderList({
      catalogData: catalog([policy()], { availabilityScope: "org_managed" }),
    });
    openPicker();
    expect(
      screen.getByText(
        "Availability checked against managed gateways; will be rechecked on deploy.",
      ),
    ).toBeInTheDocument();
  });

  it("shows hub-unreachable note when hubAvailable=false", () => {
    renderList({ catalogData: catalog([policy()], { hubAvailable: false }) });
    openPicker();
    expect(
      screen.getByText("Policy hub unreachable — showing installed policies only."),
    ).toBeInTheDocument();
  });

  it("shows no notes for a deployed scope with the hub up", () => {
    renderList({ catalogData: catalog([policy()]) });
    openPicker();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("PolicyListSection applied policies", () => {
  const applied: PolicySelection[] = [
    {
      name: "legacy-guard",
      version: "v1.0.0",
      displayName: "Legacy Guard",
      settings: { mode: "strict" },
    },
  ];

  it("applied policy missing from catalog is read-only", () => {
    const { onEdit, onRemove } = renderList(
      { catalogData: catalog([policy()]) },
      applied,
    );
    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Legacy Guard"));
    // The syntax highlighter splits tokens across spans, so read the text whole.
    expect(screen.getByRole("presentation").textContent).toContain(
      '"mode": "strict"',
    );
    expect(screen.queryByRole("button", { name: /save/i })).not.toBeInTheDocument();
    expect(onEdit).not.toHaveBeenCalled();

    const remove = screen.getByRole("button", {
      name: "Remove Legacy Guard",
      hidden: true, // the open drawer hides the list from the a11y tree
    });
    expect(remove).toBeEnabled();
    fireEvent.click(remove);
    expect(onRemove).toHaveBeenCalledWith("legacy-guard", "v1.0.0");
  });

  it("flags an applied policy whose only catalog match is not installed", () => {
    renderList(
      {
        catalogData: catalog([
          policy({
            name: "legacy-guard",
            version: "v1.0.0",
            status: "not_installed",
            parameters: undefined,
          }),
        ]),
      },
      applied,
    );
    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();
  });

  it("does not flag an applied policy that matches an installed item", () => {
    renderList(
      { catalogData: catalog([policy({ name: "legacy-guard", version: "v1.0.0" })]) },
      applied,
    );
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
  });

  it("flags an applied policy when only a different major is installed (default match is name + major)", () => {
    renderList(
      { catalogData: catalog([policy({ name: "legacy-guard", version: "v2.0.0" })]) },
      applied,
    );
    expect(screen.getByText("Not available on this gateway")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Legacy Guard"));
    expect(screen.queryByRole("button", { name: /save/i })).not.toBeInTheDocument();
  });

  it("does not flag an applied policy when a same-major version is installed", () => {
    renderList(
      { catalogData: catalog([policy({ name: "legacy-guard", version: "v1.2.0" })]) },
      applied,
    );
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
  });

  it("matches applied policies with matchApplied (name + major) and opens the inline form", () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const major = (v: string) => v.replace(/^v/, "").split(".")[0];
    renderList(
      {
        catalogData: catalog([
          policy({
            name: "legacy-guard",
            version: "v1.0.2",
            parameters: {
              type: "object",
              properties: { mode: { type: "string" } },
            },
          }),
        ]),
        matchApplied: (a, item) =>
          a.name === item.name && major(a.version) === major(item.version),
      },
      applied,
    );
    expect(
      screen.queryByText("Not available on this gateway"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Legacy Guard"));
    expect(screen.getByDisplayValue("strict")).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });

  it("edits an applied v1.0.0 policy against the installed v1.2.0 schema and keeps v1.0.0 (default matcher)", () => {
    const { onEdit } = renderList(
      {
        catalogData: catalog([
          policy({
            name: "legacy-guard",
            version: "v1.2.0",
            parameters: {
              type: "object",
              properties: { mode: { type: "string" } },
            },
          }),
        ]),
      },
      applied,
    );
    fireEvent.click(screen.getByText("Legacy Guard"));
    // The editor opens prefilled, not the picker list.
    const field = screen.getByDisplayValue("strict");
    fireEvent.change(field, { target: { value: "lenient" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onEdit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "legacy-guard",
        version: "v1.0.0",
        settings: { mode: "lenient" },
      }),
    );
  });

  it("MCP name+major editing still saves the saved major", () => {
    const major = (v: string) => v.replace(/^v/, "").split(".")[0];
    const { onEdit } = renderList(
      {
        catalogData: catalog([
          policy({
            name: "legacy-guard",
            version: "v1.0.2",
            parameters: {
              type: "object",
              properties: { mode: { type: "string" } },
            },
          }),
        ]),
        matchApplied: (a, item) =>
          a.name === item.name && major(a.version) === major(item.version),
      },
      [{ ...applied[0], version: "v1" }],
    );
    fireEvent.click(screen.getByText("Legacy Guard"));
    fireEvent.change(screen.getByDisplayValue("strict"), {
      target: { value: "lenient" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onEdit).toHaveBeenCalledWith(
      expect.objectContaining({ name: "legacy-guard", version: "v1", settings: { mode: "lenient" } }),
    );
  });
});
