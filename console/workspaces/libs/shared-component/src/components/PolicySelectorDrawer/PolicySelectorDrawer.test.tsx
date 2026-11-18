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
import { screen } from "@testing-library/react";
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
});
