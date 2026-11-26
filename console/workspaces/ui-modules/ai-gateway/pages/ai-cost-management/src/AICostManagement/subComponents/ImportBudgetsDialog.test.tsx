/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  BudgetImportResult,
  BudgetImportRowResult,
} from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  access: new Set<string>(),
  checkMutate: vi.fn(),
  saveMutate: vi.fn(),
  checkReset: vi.fn(),
  saveReset: vi.fn(),
  check: { isPending: false, error: null as unknown },
  save: { isPending: false, error: null as unknown },
  checkResult: undefined as unknown,
  saveResult: undefined as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  isNetworkError: (e: unknown) =>
    (e as { network?: boolean })?.network === true,
  useCheckBudgetImport: () => ({
    mutate: (vars: unknown, opts: { onSuccess: (r: unknown) => void }) => {
      hoisted.checkMutate(vars);
      if (hoisted.checkResult) opts.onSuccess(hoisted.checkResult);
    },
    reset: hoisted.checkReset,
    ...hoisted.check,
  }),
  useImportBudgets: () => ({
    mutate: (vars: unknown, opts: { onSuccess: (r: unknown) => void }) => {
      hoisted.saveMutate(vars);
      if (hoisted.saveResult) opts.onSuccess(hoisted.saveResult);
    },
    reset: hoisted.saveReset,
    ...hoisted.save,
  }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({
    hasAccess: (feature: string) => hoisted.access.has(feature),
  }),
}));

import { ImportBudgetsDialog } from "./ImportBudgetsDialog";

vi.setConfig({ testTimeout: 60_000 });

const CSV = [
  "name,displayName,amount,period,action",
  "finance-monthly,Finance monthly,1500.00,month,block",
  "marketing-weekly,Marketing weekly,250.00,week,allow",
  "",
].join("\n");

const csvFile = (name = "budgets.csv", content = CSV) => {
  const file = new File([content], name, { type: "text/csv" });
  // jsdom's File has no text().
  Object.defineProperty(file, "text", {
    value: () => Promise.resolve(content),
  });
  return file;
};

const result = (
  rows: BudgetImportRowResult[],
  over: Partial<BudgetImportResult["summary"]> = {},
  applied = false,
): BudgetImportResult => ({
  dryRun: !applied,
  applied,
  summary: {
    valid: rows.filter((r) => !r.errors?.length).length,
    error: rows.filter((r) => r.errors?.length).length,
    created: rows.filter((r) => !r.errors?.length && r.action === "create")
      .length,
    updated: rows.filter((r) => r.action === "update").length,
    skipped: rows.filter((r) => r.action === "skip").length,
    ...over,
  },
  results: rows,
});

const goodRows: BudgetImportRowResult[] = [
  { row: 2, name: "finance-monthly", action: "create" },
  { row: 3, name: "marketing-weekly", action: "create" },
];

const renderDialog = (
  props: Partial<React.ComponentProps<typeof ImportBudgetsDialog>> = {},
) => {
  const onClose = vi.fn();
  const view = render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <ImportBudgetsDialog open orgName="acme" onClose={onClose} {...props} />
      </MemoryRouter>
    </ThemeProvider>,
  );
  return { onClose, ...view };
};

const fileInput = () =>
  document.querySelector('input[type="file"]') as HTMLInputElement;

const pick = async (file = csvFile()) => {
  fireEvent.change(fileInput(), { target: { files: [file] } });
  await waitFor(() => expect(hoisted.checkMutate).toHaveBeenCalled());
};

const importButton = () =>
  screen.getByRole("button", { name: "Import budgets" });

beforeEach(() => {
  for (const m of [
    hoisted.checkMutate,
    hoisted.saveMutate,
    hoisted.checkReset,
    hoisted.saveReset,
  ])
    m.mockClear();
  hoisted.access = new Set(["cost_write"]);
  hoisted.check = { isPending: false, error: null };
  hoisted.save = { isPending: false, error: null };
  hoisted.checkResult = result(goodRows);
  hoisted.saveResult = undefined;
});

describe("ImportBudgetsDialog", () => {
  it("starts empty with a template link and Import disabled", () => {
    renderDialog();
    expect(
      screen.getByText("Import budgets", { selector: "h2" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Create several budgets at once from a CSV file."),
    ).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Download CSV template" });
    expect(link).toHaveAttribute("download", "budgets-template.csv");
    expect(link.getAttribute("href")).toContain("org-monthly-ceiling");
    expect(
      screen.getByRole("button", { name: "Select CSV file" }),
    ).toBeEnabled();
    expect(importButton()).toBeDisabled();
    expect(screen.queryByText("Validation passed.")).not.toBeInTheDocument();
  });

  it("opens the file chooser from the select button", () => {
    renderDialog();
    const click = vi.spyOn(fileInput(), "click");
    fireEvent.click(screen.getByRole("button", { name: "Select CSV file" }));
    expect(click).toHaveBeenCalled();
  });

  it("checks the chosen file and lists each row ready to create", async () => {
    renderDialog();
    await pick();
    expect(hoisted.checkMutate).toHaveBeenCalledWith({
      csv: CSV,
      onExisting: "fail",
    });
    expect(await screen.findByText("budgets.csv")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Replace file" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Validation passed.")).toBeInTheDocument();
    expect(
      screen.getByText("All 2 budgets are ready to import."),
    ).toBeInTheDocument();
    const row = screen
      .getByText("finance-monthly")
      .closest("tr") as HTMLElement;
    expect(within(row).getByText("Ready to create")).toBeInTheDocument();
    expect(importButton()).toBeEnabled();
  });

  it("says a single valid budget is ready", async () => {
    hoisted.checkResult = result([goodRows[0]]);
    renderDialog();
    await pick();
    expect(
      await screen.findByText("The budget is ready to import."),
    ).toBeInTheDocument();
  });

  it("shows row errors with their column and blocks the import", async () => {
    hoisted.checkResult = result([
      goodRows[0],
      {
        row: 3,
        errors: [
          {
            column: "amount",
            code: "INVALID_AMOUNT",
            message: "Amount must be above 0.",
          },
          { code: "INVALID_ROW", message: "Row is incomplete." },
        ],
      },
    ]);
    renderDialog();
    await pick();
    expect(await screen.findByText("Validation failed.")).toBeInTheDocument();
    expect(
      screen.getByText("1 of 2 budgets have errors. Nothing was imported."),
    ).toBeInTheDocument();
    const row = screen.getByText("Line 3").closest("tr") as HTMLElement;
    expect(within(row).getByText("Invalid")).toBeInTheDocument();
    expect(
      within(row).getByText(/Amount must be above 0\./),
    ).toBeInTheDocument();
    expect(within(row).getByText("amount")).toBeInTheDocument();
    expect(within(row).getByText("Row is incomplete.")).toBeInTheDocument();
    expect(importButton()).toBeDisabled();
  });

  it("explains a taken name as a choice of skip or update", async () => {
    hoisted.checkResult = result([
      {
        row: 2,
        name: "finance-monthly",
        errors: [
          {
            column: "name",
            code: "BUDGET_ALREADY_EXISTS",
            message: "budget exists, use onExisting=skip",
          },
        ],
      },
    ]);
    renderDialog();
    await pick();
    expect(
      await screen.findByText(
        /Name taken\. Choose skip or update for existing budgets\./,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/onExisting/)).not.toBeInTheDocument();
  });

  it("reads Nothing to import when every budget would be skipped", async () => {
    hoisted.checkResult = result(
      [{ row: 2, name: "finance-monthly", action: "skip" }],
      { created: 0, updated: 0 },
    );
    renderDialog();
    await pick();
    expect(await screen.findByText("Nothing to import.")).toBeInTheDocument();
    expect(screen.getByText("Will be skipped")).toBeInTheDocument();
    expect(importButton()).toBeDisabled();
  });

  it("marks rows that update an existing budget", async () => {
    hoisted.checkResult = result([
      { row: 2, name: "finance-monthly", action: "update" },
    ]);
    renderDialog();
    await pick();
    expect(await screen.findByText("Ready to update")).toBeInTheDocument();
    expect(importButton()).toBeEnabled();
  });

  it("rejects a file over 1 MB without checking it", async () => {
    renderDialog();
    const big = csvFile("all-teams.csv");
    Object.defineProperty(big, "size", { value: 1024 * 1024 + 1 });
    fireEvent.change(fileInput(), { target: { files: [big] } });
    expect(
      await screen.findByText("The file must be at most 1 MB."),
    ).toBeInTheDocument();
    expect(hoisted.checkMutate).not.toHaveBeenCalled();
    expect(importButton()).toBeDisabled();
  });

  it("ignores a cancelled file chooser", () => {
    renderDialog();
    fireEvent.change(fileInput(), { target: { files: [] } });
    expect(hoisted.checkMutate).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Select CSV file" }),
    ).toBeInTheDocument();
  });

  it("shows Validating while the check runs", async () => {
    hoisted.check = { isPending: true, error: null };
    hoisted.checkResult = undefined;
    renderDialog();
    expect(screen.getByText("Validating…")).toBeInTheDocument();
    expect(importButton()).toBeDisabled();
  });

  it("shows the server's message when it refuses the file", () => {
    hoisted.check = {
      isPending: false,
      error: { message: "INVALID_CSV: missing column amount" },
    };
    renderDialog();
    expect(
      screen.getByText("INVALID_CSV: missing column amount"),
    ).toBeInTheDocument();
  });

  it("shows a generic message for a bare HTTP error", () => {
    hoisted.check = { isPending: false, error: { message: "HTTP error 500" } };
    renderDialog();
    expect(
      screen.getByText("Couldn't check the file. Try again."),
    ).toBeInTheDocument();
  });

  it("shows the size message for a 413", () => {
    hoisted.save = { isPending: false, error: { status: 413 } };
    renderDialog();
    expect(
      screen.getByText("The file must be at most 1 MB."),
    ).toBeInTheDocument();
  });

  it("shows an unreachable-server message for a network error", () => {
    hoisted.check = { isPending: false, error: { network: true } };
    renderDialog();
    expect(
      screen.getByText("The server can't be reached. Try again."),
    ).toBeInTheDocument();
  });

  it("imports the file and closes when the budgets were saved", async () => {
    hoisted.saveResult = result(goodRows, {}, true);
    const { onClose } = renderDialog();
    await pick();
    fireEvent.click(
      await screen.findByRole("button", { name: "Import budgets" }),
    );
    expect(hoisted.saveMutate).toHaveBeenCalledWith({
      csv: CSV,
      onExisting: "fail",
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(hoisted.checkReset).toHaveBeenCalled();
    expect(hoisted.saveReset).toHaveBeenCalled();
  });

  it("shows the new errors when the data changed since the check", async () => {
    hoisted.saveResult = result(
      [
        {
          row: 2,
          name: "finance-monthly",
          errors: [
            {
              column: "name",
              code: "BUDGET_ALREADY_EXISTS",
              message: "exists",
            },
          ],
        },
      ],
      {},
      false,
    );
    const { onClose } = renderDialog();
    await pick();
    fireEvent.click(
      await screen.findByRole("button", { name: "Import budgets" }),
    );
    expect(await screen.findByText("Validation failed.")).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("checks the file again with the new mode when duplicate handling changes", async () => {
    renderDialog();
    await pick();
    hoisted.checkMutate.mockClear();
    fireEvent.mouseDown(
      document.getElementById("duplicate-budget-handling") as HTMLElement,
    );
    fireEvent.click(
      screen.getByRole("option", { name: "Skip existing budgets" }),
    );
    expect(hoisted.checkMutate).toHaveBeenCalledWith({
      csv: CSV,
      onExisting: "skip",
    });
    fireEvent.click(
      await screen.findByRole("button", { name: "Import budgets" }),
    );
    expect(hoisted.saveMutate).toHaveBeenCalledWith({
      csv: CSV,
      onExisting: "skip",
    });
  });

  it("changing the mode before a file is chosen does not check anything", () => {
    renderDialog();
    fireEvent.mouseDown(
      document.getElementById("duplicate-budget-handling") as HTMLElement,
    );
    fireEvent.click(
      screen.getByRole("option", { name: "Skip existing budgets" }),
    );
    expect(hoisted.checkMutate).not.toHaveBeenCalled();
  });

  it("offers Update only with permission to edit budgets", () => {
    hoisted.access = new Set();
    renderDialog();
    expect(
      screen.getByText("Needs permission to edit budgets."),
    ).toBeInTheDocument();
    fireEvent.mouseDown(
      document.getElementById("duplicate-budget-handling") as HTMLElement,
    );
    expect(
      screen.getByRole("option", { name: "Update existing budgets" }),
    ).toHaveAttribute("aria-disabled", "true");
  });

  it("allows Update with cost_write", () => {
    renderDialog();
    expect(
      screen.queryByText("Needs permission to edit budgets."),
    ).not.toBeInTheDocument();
    fireEvent.mouseDown(
      document.getElementById("duplicate-budget-handling") as HTMLElement,
    );
    expect(
      screen.getByRole("option", { name: "Update existing budgets" }),
    ).not.toHaveAttribute("aria-disabled", "true");
  });

  it("closes with Cancel and the close button, resetting its state", () => {
    const { onClose } = renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
    expect(hoisted.checkReset).toHaveBeenCalled();
  });

  it("cannot be closed or changed while the import is running", () => {
    hoisted.save = { isPending: true, error: null };
    const { onClose } = renderDialog();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Select CSV file" }),
    ).toBeDisabled();
    expect(importButton()).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).not.toHaveBeenCalled();
  });
});
