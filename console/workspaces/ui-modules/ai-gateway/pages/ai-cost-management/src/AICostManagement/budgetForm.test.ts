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

import {
  ANY,
  budgetFromForm,
  EMPTY_FORM,
  AMOUNT_PATTERN,
  exemptionKinds,
  duplicateNameError,
  formFieldOf,
  formFor,
  formFromBudget,
  isBudgetNotFound,
  handleFor,
  newExemption,
  newFilter,
  sentFilterRows,
  validateBudgetForm,
  type BudgetFormState,
} from "./budgetForm";

const form = (over: Partial<BudgetFormState>): BudgetFormState => ({
  ...EMPTY_FORM,
  displayName: "Payments on OpenAI",
  amount: "1000",
  ...over,
});

describe("budgetForm", () => {
  it("derives a handle from the name and numbers it while taken", () => {
    expect(handleFor("Payments on OpenAI!", [])).toBe("payments-on-openai");
    expect(handleFor("Payments", ["payments", "payments-2"])).toBe(
      "payments-3",
    );
    expect(handleFor("   ", [])).toBe("budget");
    expect(handleFor("x".repeat(80), []).length).toBeLessThanOrEqual(50);
  });

  it("sends filters and the split, and reads them back", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          { ...newFilter("llmProvider"), id: "mock-openai" },
          { ...newFilter("agent"), id: "checkout", project: "payments" },
          {
            ...newFilter("team"),
            expression: "'R&D' in request.Auth.Claims[\"groups\"]",
          },
        ],
        splitBy: "user",
        splitExpression: 'request.Auth.Claims["email"]',
      }),
      { name: "payments" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "llmProvider", id: "mock-openai" },
      { type: "agent", project: "payments", id: "checkout" },
      {
        type: "attribute",
        expression: "'R&D' in request.Auth.Claims[\"groups\"]",
      },
    ]);
    expect(budget.spec.limit.splitBy).toEqual({
      type: "attribute",
      expression: 'request.Auth.Claims["email"]',
    });
    const back = formFromBudget(budget);
    expect(back.filters.map((f) => [f.type, f.id, f.project])).toEqual([
      ["llmProvider", "mock-openai", ""],
      ["agent", "checkout", "payments"],
      ["team", "", ""],
      ["user", "", ""],
    ]);
    expect(back.splitBy).toBe("user");
  });

  it("sends one-word labels as metadata.labels, drops blanks, and checks each word", () => {
    const created = budgetFromForm(form({ labels: ["finance", " ", "prod"] }), {
      name: "labelled",
    });
    expect(created.metadata.labels).toEqual(["finance", "prod"]);
    expect(formFromBudget(created).labels).toEqual(["finance", "prod"]);
    const edited = budgetFromForm(form({ labels: [] }), {
      name: "labelled",
      existing: created,
    });
    expect(edited.metadata.labels).toBeUndefined();
    expect(edited.metadata.name).toBe("labelled");
    const errs = validateBudgetForm(
      form({ amount: "1", displayName: "a", labels: ["ok", "-bad", "ok"] }),
    );
    expect(errs.labels).toMatch(/one word/);
    expect(
      validateBudgetForm(
        form({ amount: "1", displayName: "a", labels: ["ok", "ok"] }),
      ).labels,
    ).toMatch(/once/);
  });

  it("splits by an Applies to filter set to All", () => {
    const split = form({
      filters: [newFilter("llmProvider"), newFilter("agent")],
      splitBy: "llmProvider",
    });
    const budget = budgetFromForm(split, { name: "per-provider" });
    expect(budget.spec.filters).toEqual([{ type: "agent" }]);
    expect(budget.spec.limit.splitBy).toEqual({ type: "llmProvider" });
    expect(
      validateBudgetForm({ ...split, amount: "1", displayName: "a" }),
    ).toEqual({});
    expect(formFromBudget(budget).filters.map((f) => [f.type, f.id])).toEqual([
      ["llmProvider", ANY],
      ["agent", ANY],
    ]);
    const one = form({
      filters: [{ ...newFilter("llmProvider"), id: "openai" }],
      splitBy: "llmProvider",
      amount: "1",
      displayName: "a",
    });
    expect(validateBudgetForm(one).splitBy).toMatch(/filter to All/);
    expect(
      validateBudgetForm({ ...one, filters: [], splitBy: "agent" }).splitBy,
    ).toMatch(/Applies to/);
  });

  it("sends All agents and API keys without an id, and All LLM providers as no filter", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          newFilter("llmProvider"),
          newFilter("agent"),
          newFilter("apiKey"),
        ],
      }),
      { name: "any" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "agent" },
      { type: "apiKey" },
    ]);
    expect(formFromBudget(budget).filters.map((f) => f.id)).toEqual(["*", "*"]);
  });

  it("sends one shared limit with no split and no filters", () => {
    const budget = budgetFromForm(form({}), { name: "all" });
    expect(budget.spec.filters).toEqual([]);
    expect(budget.spec.limit.splitBy).toBeUndefined();
    expect(
      budgetFromForm(form({ splitBy: "agent" }), { name: "a" }).spec.limit
        .splitBy,
    ).toEqual({ type: "agent" });
  });

  it("starts a budget from an entity's page with that entity as a filter", () => {
    expect(
      formFor({ agent: "online-shop/returns-bot" }).filters.map((f) => [
        f.type,
        f.id,
        f.project,
      ]),
    ).toEqual([["agent", "returns-bot", "online-shop"]]);
    expect(formFor({ provider: "mock-openai" }).filters[0]).toMatchObject({
      type: "llmProvider",
      id: "mock-openai",
    });
  });

  it("owns a new budget by the project of the page, and keeps metadata on edit", () => {
    const created = budgetFromForm(form({}), {
      name: "payments",
      projectOwner: "payments",
    });
    expect(created.metadata.annotations).toEqual({
      "gateway.api-platform.wso2.com/project-id": "payments",
    });
    const edited = budgetFromForm(form({ amount: "1500" }), {
      name: "ignored",
      existing: created,
    });
    expect(edited.metadata).toEqual(created.metadata);
    expect(edited.spec.limit.amount).toBe("1500.00");
  });

  it("sends block-at only for Block, and warnings only with thresholds", () => {
    const allow = budgetFromForm(form({ action: "allow", blockAt: "90" }), {
      name: "a",
    });
    expect(allow.spec.limit.blockAt).toBeUndefined();
    expect(allow.spec.warnings).toBeUndefined();
    const warned = budgetFromForm(
      form({ thresholds: [100, 80, 80], channels: ["ops-slack"] }),
      {
        name: "a",
      },
    );
    expect(warned.spec.warnings).toEqual({
      thresholds: [80, 100],
      channels: ["ops-slack"],
    });
  });

  it("offers only exemption kinds narrower than what the budget counts", () => {
    expect(exemptionKinds({ filters: [], splitBy: "" })).toEqual([
      "project",
      "agent",
      "team",
      "user",
      "attribute",
    ]);
    expect(
      exemptionKinds({ filters: [newFilter("project")], splitBy: "" }),
    ).toEqual(["agent", "team", "user", "attribute"]);
    expect(
      exemptionKinds({ filters: [newFilter("llmProvider")], splitBy: "agent" }),
    ).toEqual(["team", "user", "attribute", "agent"]);
    expect(exemptionKinds({ filters: [], splitBy: "user" })).toEqual([
      "attribute",
      "user",
    ]);
  });

  it("flags an exemption that is not narrower", () => {
    const errors = validateBudgetForm(
      form({
        filters: [{ ...newFilter("agent"), id: "a", project: "p" }],
        exemptions: [{ ...newExemption("agent"), id: "b", project: "p" }],
      }),
    );
    expect(errors["exemptions.0"]).toMatch(/narrower/);
  });

  it("accepts thresholds up to 1000% and at most 10 of them", () => {
    expect(
      validateBudgetForm(form({ thresholds: [150, 1000], channels: ["ops"] })),
    ).toEqual({});
    expect(
      validateBudgetForm(form({ thresholds: [1001], channels: ["ops"] })),
    ).toHaveProperty("thresholds");
    const eleven = Array.from({ length: 11 }, (_, i) => (i + 1) * 10);
    expect(
      validateBudgetForm(form({ thresholds: eleven, channels: ["ops"] })),
    ).toHaveProperty("thresholds");
  });

  it("takes only amounts a gateway accepts", () => {
    for (const ok of ["1", "100", "0100", "0.5", "0.05", "999999999999.99"]) {
      expect(AMOUNT_PATTERN.test(ok), ok).toBe(true);
    }
    for (const bad of [
      "0",
      "0.00",
      "1000000000000",
      "10.005",
      "-5",
      "1e3",
      "",
    ]) {
      expect(AMOUNT_PATTERN.test(bad), bad).toBe(false);
    }
  });

  it("takes only amounts a gateway accepts", () => {
    for (const ok of ["1", "100", "0100", "0.5", "0.05", "999999999999.99"]) {
      expect(AMOUNT_PATTERN.test(ok), ok).toBe(true);
    }
    for (const bad of [
      "0",
      "0.00",
      "1000000000000",
      "10.005",
      "-5",
      "1e3",
      "",
    ]) {
      expect(AMOUNT_PATTERN.test(bad), bad).toBe(false);
    }
  });

  it("refuses a name another budget uses, but not the budget's own", () => {
    const budgets = [
      {
        metadata: { name: "shop-weekly" },
        spec: { displayName: "Shop weekly" },
      },
    ];
    expect(duplicateNameError(" shop WEEKLY ", budgets)).toBe(
      "A budget named shop WEEKLY already exists.",
    );
    expect(
      duplicateNameError("Shop weekly", budgets, "shop-weekly"),
    ).toBeUndefined();
    expect(duplicateNameError("Shop daily", budgets)).toBeUndefined();
  });

  it("tells a missing budget from a failure", () => {
    expect(isBudgetNotFound({ code: "BUDGET_NOT_FOUND", message: "x" })).toBe(
      true,
    );
    expect(isBudgetNotFound(new TypeError("Failed to fetch"))).toBe(false);
  });

  it("tells a missing budget from a failure", () => {
    expect(isBudgetNotFound({ code: "BUDGET_NOT_FOUND", message: "x" })).toBe(
      true,
    );
    expect(isBudgetNotFound(new TypeError("Failed to fetch"))).toBe(false);
  });

  it("maps the server's field names onto the form's fields", () => {
    expect(formFieldOf("spec.limit.amount")).toBe("amount");
    expect(formFieldOf("warnings.channels")).toBe("channels");
    expect(formFieldOf("warnings.thresholds[0]")).toBe("thresholds");
    expect(formFieldOf("spec.exemptions[1].expression")).toBe("exemptions.1");
    expect(formFieldOf("spec.filters[2].id")).toBe("filters.2");
    expect(formFieldOf("spec.filters")).toBe("filters");
    expect(formFieldOf("spec.limit.splitBy.expression")).toBe("splitBy");
    expect(formFieldOf("metadata.annotations")).toBe("form");
    expect(formFieldOf(undefined)).toBe("form");
  });

  it("checks what the server would refuse", () => {
    expect(validateBudgetForm(form({}))).toEqual({});
    const errors = validateBudgetForm(
      form({
        displayName: " ",
        amount: "10.005",
        blockAt: "0",
        thresholds: [80],
        filters: [{ ...newFilter("llmProvider"), id: "" }, newFilter("team")],
        splitBy: "attribute",
        splitExpression: " ",
      }),
    );
    expect(Object.keys(errors).sort()).toEqual(
      [
        "amount",
        "blockAt",
        "channels",
        "displayName",
        "filters.0",
        "filters.1",
        "splitBy",
      ].sort(),
    );
  });

  it("points a server error on a sent filter at its form row", () => {
    const rows = [
      { type: "llmProvider" as const, id: ANY, project: "", expression: "" },
      {
        type: "project" as const,
        id: "online-shop",
        project: "",
        expression: "",
      },
      {
        type: "agent" as const,
        id: "bad",
        project: "online-shop",
        expression: "",
      },
    ];
    // All LLM providers is not sent, so the request's filters[1] is the agent (row 2).
    expect(formFieldOf("spec.filters[1].id", sentFilterRows(rows))).toBe(
      "filters.2",
    );
    expect(formFieldOf("spec.filters[0].id", sentFilterRows(rows))).toBe(
      "filters.1",
    );
    // A filter the form did not send (added by the server) points at the filters.
    expect(formFieldOf("spec.filters[5].id", sentFilterRows(rows))).toBe(
      "filters",
    );
  });
});
