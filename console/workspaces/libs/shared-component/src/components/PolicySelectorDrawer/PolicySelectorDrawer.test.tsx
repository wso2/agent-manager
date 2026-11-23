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
import { policy, renderWithProviders } from "./__test__/helpers";

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

import { PolicySelectorDrawer } from "./PolicySelectorDrawer";

const major = (v: string) => v.replace(/^v/, "").split(".")[0];

describe("PolicySelectorDrawer", () => {
  it("MCP applied v1 matches catalog v1.0.2 and opens inline form", () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    renderWithProviders(
      <PolicySelectorDrawer
        open
        onClose={vi.fn()}
        onSubmit={vi.fn()}
        filterPolicies={(items) => items}
        catalogData={{
          count: 1,
          hubAvailable: true,
          availabilityScope: "deployed",
          availabilityKnown: true,
          data: [policy({ version: "v1.0.2" })],
        }}
        editPolicyKey="pii-masking@v1"
        matchEdit={(item) => item.name === "pii-masking" && major(item.version) === "1"}
        existingSettings={{ blockedWord: "secret" }}
      />,
    );
    expect(screen.getByDisplayValue("secret")).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });

  describe("catalog changes while a policy is selected", () => {
    const catalogOf = (...data: ReturnType<typeof policy>[]) => ({
      count: data.length,
      hubAvailable: true,
      availabilityScope: "deployed" as const,
      availabilityKnown: true,
      data,
    });
    const editProps = {
      open: true,
      onClose: vi.fn(),
      filterPolicies: (items: ReturnType<typeof policy>[]) => items,
      editPolicyKey: "pii-masking@v1",
      matchEdit: (item: ReturnType<typeof policy>) =>
        item.name === "pii-masking" && major(item.version) === "1",
      existingSettings: { blockedWord: "secret" },
    };
    const WARNING = /not available on this gateway/i;

    it("edit mode: keeps edits, warns and disables Save when the policy turns not_installed, then recovers", () => {
      const onSubmit = vi.fn();
      const { rerender } = renderWithProviders(
        <PolicySelectorDrawer
          {...editProps}
          onSubmit={onSubmit}
          catalogData={catalogOf(policy({ version: "v1.0.2" }))}
        />,
      );
      fireEvent.change(screen.getByDisplayValue("secret"), {
        target: { value: "edited" },
      });
      expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
      expect(screen.queryByText(WARNING)).not.toBeInTheDocument();

      rerender(
        <PolicySelectorDrawer
          {...editProps}
          onSubmit={onSubmit}
          catalogData={catalogOf(
            policy({ version: "v1.0.2", status: "not_installed", parameters: undefined }),
          )}
        />,
      );
      expect(screen.getByDisplayValue("edited")).toBeInTheDocument();
      expect(screen.getByText(WARNING)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

      rerender(
        <PolicySelectorDrawer
          {...editProps}
          onSubmit={onSubmit}
          catalogData={catalogOf(policy({ version: "v1.0.3" }))}
        />,
      );
      expect(screen.getByDisplayValue("edited")).toBeInTheDocument();
      expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    });

    it("edit mode: disables Save when the policy is removed from the catalog", () => {
      const { rerender } = renderWithProviders(
        <PolicySelectorDrawer
          {...editProps}
          onSubmit={vi.fn()}
          catalogData={catalogOf(policy({ version: "v1.0.2" }))}
        />,
      );
      rerender(
        <PolicySelectorDrawer {...editProps} onSubmit={vi.fn()} catalogData={catalogOf()} />,
      );
      expect(screen.getByDisplayValue("secret")).toBeInTheDocument();
      expect(screen.getByText(WARNING)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    });

    it("add mode: disables Add when the selected policy turns not_installed", () => {
      const props = {
        open: true,
        onClose: vi.fn(),
        onSubmit: vi.fn(),
        filterPolicies: (items: ReturnType<typeof policy>[]) => items,
      };
      const { rerender } = renderWithProviders(
        <PolicySelectorDrawer {...props} catalogData={catalogOf(policy())} />,
      );
      fireEvent.click(screen.getByText("PII Masking"));
      expect(screen.getByRole("button", { name: "Add" })).toBeEnabled();

      rerender(
        <PolicySelectorDrawer
          {...props}
          catalogData={catalogOf(policy({ status: "not_installed", parameters: undefined }))}
        />,
      );
      expect(screen.getByText(WARNING)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Add" })).toBeDisabled();

      rerender(<PolicySelectorDrawer {...props} catalogData={catalogOf(policy())} />);
      expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Add" })).toBeEnabled();
    });
  });

  describe("availability unknown", () => {
    const UNKNOWN = "This gateway hasn't reported its policies yet — availability will show once it does.";
    const unknownCatalog = (...data: ReturnType<typeof policy>[]) => ({
      count: data.length,
      hubAvailable: true,
      availabilityScope: "org_managed" as const,
      availabilityKnown: false,
      data,
    });
    const notInstalled = (extra = {}) =>
      policy({ status: "not_installed", parameters: undefined, ...extra });

    it("shows the note at the top of the list and keeps not-installed rows disabled", () => {
      renderWithProviders(
        <PolicySelectorDrawer
          open
          onClose={vi.fn()}
          onSubmit={vi.fn()}
          filterPolicies={(items) => items}
          catalogData={unknownCatalog(notInstalled())}
        />,
      );
      expect(screen.getByText(UNKNOWN)).toBeInTheDocument();
      expect(screen.getByText("Availability not known yet")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /PII Masking/ })).toHaveAttribute("aria-disabled", "true");
    });

    it("edit mode: keeps Save enabled for a not-installed entry that carries a schema", () => {
      renderWithProviders(
        <PolicySelectorDrawer
          open
          onClose={vi.fn()}
          onSubmit={vi.fn()}
          filterPolicies={(items) => items}
          editPolicyKey="pii-masking@v1"
          matchEdit={(item) => item.name === "pii-masking"}
          existingSettings={{ blockedWord: "secret" }}
          catalogData={unknownCatalog(
            notInstalled({ parameters: { type: "object", properties: { blockedWord: { type: "string" } } } }),
          )}
        />,
      );
      expect(screen.getByDisplayValue("secret")).toBeInTheDocument();
      expect(screen.queryByText(/not available on this gateway/i)).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    });
  });
});
