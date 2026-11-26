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

import { describe, expect, it } from "vitest";
import {
  IMPORT_TEMPLATE,
  importErrorMessage,
  rowError,
  rowStatus,
  validationResult,
} from "./budgetImport";

describe("budgetImport", () => {
  it("shows a validation status, never an action already taken", () => {
    expect(rowStatus({ row: 2, action: "create" })).toEqual({
      valid: true,
      label: "Ready to create",
    });
    expect(rowStatus({ row: 3, action: "update" }).label).toBe(
      "Ready to update",
    );
    expect(rowStatus({ row: 4, action: "skip" }).label).toBe("Will be skipped");
    expect(
      rowStatus({
        row: 5,
        errors: [{ code: "VALIDATION_ERROR", message: "x" }],
      }),
    ).toEqual({
      valid: false,
      label: "Invalid",
    });
  });

  it("keeps the CSV column apart from the message", () => {
    expect(
      rowError({
        code: "REFERENCE_NOT_FOUND",
        column: "project",
        message: "Project 'api-platform' was not found.",
      }),
    ).toEqual({
      column: "project",
      message: "Project 'api-platform' was not found.",
    });
    expect(
      rowError({
        code: "BUDGET_ALREADY_EXISTS",
        column: "name",
        message: "use onExisting=update",
      }).message,
    ).not.toMatch(/onExisting/);
  });

  it("summarises validation compactly; imports are atomic", () => {
    const base = { valid: 0, error: 0, created: 0, updated: 0, skipped: 0 };
    expect(
      validationResult({ ...base, valid: 1, error: 1, created: 1 }, 2),
    ).toEqual({
      severity: "error",
      title: "Validation failed.",
      text: "1 of 2 budgets have errors. Nothing was imported.",
    });
    expect(validationResult({ ...base, valid: 2, created: 2 }, 2).text).toBe(
      "All 2 budgets are ready to import.",
    );
    expect(
      validationResult({ ...base, valid: 1, skipped: 1 }, 1).severity,
    ).toBe("info");
  });

  it("uses the server's message for a refused file", () => {
    expect(
      importErrorMessage({
        code: "INVALID_CSV",
        message: "The header has no name column.",
      }),
    ).toBe("The header has no name column.");
    expect(importErrorMessage({ status: 413 })).toBe(
      "The file must be at most 1 MB.",
    );
    expect(importErrorMessage(new Error("HTTP error! status: 500"))).toBe(
      "Couldn't check the file. Try again.",
    );
  });

  it("offers a template with every column and the same count in each row", () => {
    const [header, ...rows] = IMPORT_TEMPLATE.trim().split("\n");
    expect(header.split(",")).toHaveLength(18);
    rows.forEach((r) => expect(r.split(",")).toHaveLength(18));
  });
});
