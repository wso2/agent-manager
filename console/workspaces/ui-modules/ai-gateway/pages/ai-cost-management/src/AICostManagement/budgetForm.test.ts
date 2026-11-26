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
  addExpressions,
  budgetFromForm,
  EMPTY_FORM,
  AMOUNT_PATTERN,
  exemptionKinds,
  duplicateNameError,
  formFieldOf,
  formFor,
  groupFilterEntries,
  formFromBudget,
  joinExpressions,
  splitExpressions,
  isBudgetNotFound,
  handleFor,
  newExemption,
  sentExemptionRows,
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
          { ...newFilter("llmProvider"), values: ["mock-openai"] },
          { ...newFilter("agent"), values: ["payments/checkout"] },
          {
            ...newFilter("team"),
            values: ["'R&D' in request.Auth.Claims[\"groups\"]"],
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
    expect(back.filters.map((f) => [f.type, f.values])).toEqual([
      ["llmProvider", ["mock-openai"]],
      ["agent", ["payments/checkout"]],
      ["team", ["'R&D' in request.Auth.Claims[\"groups\"]"]],
      ["user", []],
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
    expect(
      formFromBudget(budget).filters.map((f) => [f.type, f.values]),
    ).toEqual([
      ["llmProvider", [ANY]],
      ["agent", [ANY]],
    ]);
    const one = form({
      filters: [{ ...newFilter("llmProvider"), values: ["openai"] }],
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
    expect(formFromBudget(budget).filters.map((f) => f.values)).toEqual([
      [ANY],
      [ANY],
    ]);
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
        f.values,
      ]),
    ).toEqual([["agent", ["online-shop/returns-bot"]]]);
    expect(formFor({ provider: "mock-openai" }).filters[0]).toMatchObject({
      type: "llmProvider",
      values: ["mock-openai"],
    });
  });

  it("names a new budget with no owner project, and keeps metadata on edit", () => {
    const created = budgetFromForm(form({}), { name: "payments" });
    expect(created.metadata).toEqual({ name: "payments" });
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

  it("offers every kind for an exemption, whatever the budget counts", () => {
    const all = [
      "llmProvider",
      "project",
      "agent",
      "apiKey",
      "team",
      "user",
      "attribute",
    ];
    expect(exemptionKinds()).toEqual(all);
  });

  it("sends an LLM provider or API key exemption by its id, and reads it back", () => {
    const budget = budgetFromForm(
      form({
        exemptions: [
          { ...newExemption("llmProvider"), values: ["azure-openai-eu"] },
          { ...newExemption("apiKey"), values: ["team-alpha-key"] },
        ],
      }),
      { name: "skip" },
    );
    expect(budget.spec.exemptions).toEqual([
      { type: "llmProvider", id: "azure-openai-eu" },
      { type: "apiKey", id: "team-alpha-key" },
    ]);
    expect(
      formFromBudget(budget).exemptions.map((e) => [e.type, e.values]),
    ).toEqual([
      ["llmProvider", ["azure-openai-eu"]],
      ["apiKey", ["team-alpha-key"]],
    ]);
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
      expect(AMOUNT_PATTERN.test(ok)).toBe(true);
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
      expect(AMOUNT_PATTERN.test(bad)).toBe(false);
    }
  });

  it("takes only amounts a gateway accepts", () => {
    for (const ok of ["1", "100", "0100", "0.5", "0.05", "999999999999.99"]) {
      expect(AMOUNT_PATTERN.test(ok)).toBe(true);
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
      expect(AMOUNT_PATTERN.test(bad)).toBe(false);
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
        filters: [
          { ...newFilter("llmProvider"), values: [] },
          newFilter("team"),
        ],
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
      { type: "llmProvider" as const, values: [ANY] },
      { type: "project" as const, values: ["online-shop"] },
      { type: "agent" as const, values: ["online-shop/bad"] },
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

  it("sends several values of a type as repeated entries, and reads them back as one row", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          { ...newFilter("project"), values: ["project-a", "project-b"] },
          { ...newFilter("llmProvider"), values: ["openai"] },
          {
            ...newFilter("agent"),
            values: ["project-a/checkout", "project-b/returns-bot"],
          },
          { ...newFilter("apiKey"), values: ["key-1", "key-2", "key-3"] },
        ],
      }),
      { name: "several" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "project", id: "project-a" },
      { type: "project", id: "project-b" },
      { type: "llmProvider", id: "openai" },
      { type: "agent", project: "project-a", id: "checkout" },
      { type: "agent", project: "project-b", id: "returns-bot" },
      { type: "apiKey", id: "key-1" },
      { type: "apiKey", id: "key-2" },
      { type: "apiKey", id: "key-3" },
    ]);
    const back = formFromBudget(budget);
    expect(back.filters.map((f) => [f.type, f.values])).toEqual([
      ["project", ["project-a", "project-b"]],
      ["llmProvider", ["openai"]],
      ["agent", ["project-a/checkout", "project-b/returns-bot"]],
      ["apiKey", ["key-1", "key-2", "key-3"]],
    ]);
    // Saving what was read back sends the same filters.
    expect(budgetFromForm(back, { name: "several" }).spec.filters).toEqual(
      budget.spec.filters,
    );
    expect(
      validateBudgetForm({ ...back, amount: "1", displayName: "a" }),
    ).toEqual({});
  });

  it("sends a Team, User or custom attribute row as one entry whose expressions are ORed, and reads it back as one row", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          {
            ...newFilter("team"),
            values: [
              "'R&D' in request.Auth.Claims[\"groups\"]",
              "'QA' in request.Auth.Claims[\"groups\"]",
            ],
          },
          {
            ...newFilter("attribute"),
            values: ['request.Headers["x-cost-center"] == "finance"'],
          },
        ],
      }),
      { name: "cel" },
    );
    expect(budget.spec.filters).toEqual([
      {
        type: "attribute",
        expression:
          "('R&D' in request.Auth.Claims[\"groups\"]) || ('QA' in request.Auth.Claims[\"groups\"])",
      },
      {
        type: "attribute",
        expression: 'request.Headers["x-cost-center"] == "finance"',
      },
    ]);
    const back = formFromBudget(budget);
    expect(back.filters.map((f) => [f.type, f.values])).toEqual([
      [
        "team",
        [
          "'R&D' in request.Auth.Claims[\"groups\"]",
          "'QA' in request.Auth.Claims[\"groups\"]",
        ],
      ],
      ["attribute", ['request.Headers["x-cost-center"] == "finance"']],
    ]);
    expect(budgetFromForm(back, { name: "cel" }).spec.filters).toEqual(
      budget.spec.filters,
    );
  });

  it("joins expressions with || and splits them back, leaving other expressions alone", () => {
    expect(joinExpressions(["a == 1"])).toBe("a == 1");
    expect(joinExpressions(["a == 1", " b == 2 ", ""])).toBe(
      "(a == 1) || (b == 2)",
    );
    expect(splitExpressions("(a == 1) || (b == 2)")).toEqual([
      "a == 1",
      "b == 2",
    ]);
    expect(splitExpressions("a == 1")).toEqual(["a == 1"]);
    expect(splitExpressions("(a == 1)")).toEqual(["(a == 1)"]);
    expect(splitExpressions("(a) && (b) || (c)")).toEqual([
      "(a) && (b) || (c)",
    ]);
    expect(splitExpressions("(x == ')') || (y)")).toEqual(["x == ')'", "y"]);
  });

  it("keeps several custom attribute rows as separate filters", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          { ...newFilter("attribute"), values: ["a == 1", "b == 2"] },
          { ...newFilter("attribute"), values: ["c == 3"] },
        ],
      }),
      { name: "two-custom" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "attribute", expression: "(a == 1) || (b == 2)" },
      { type: "attribute", expression: "c == 3" },
    ]);
    expect(
      formFromBudget(budget).filters.map((f) => [f.type, f.values]),
    ).toEqual([
      ["attribute", ["a == 1", "b == 2"]],
      ["attribute", ["c == 3"]],
    ]);
  });

  it("trims only the ends of a typed expression, and refuses repeats and long ones", () => {
    expect(
      addExpressions([], '  request.Auth.Claims["email"] == "a b"  '),
    ).toEqual({
      values: ['request.Auth.Claims["email"] == "a b"'],
    });
    expect(addExpressions(["a"], "   ")).toEqual({ values: ["a"] });
    expect(addExpressions(["a"], " a ").error).toBe(
      "Each expression can be added once.",
    );
    expect(addExpressions(["a"], "x".repeat(1025)).error).toMatch(
      /at most 1024/,
    );
    expect(addExpressions(["a"], ["b", "c"]).values).toEqual(["a", "b", "c"]);
  });

  it("writes a single expression exactly as before", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          {
            ...newFilter("user"),
            values: ['request.Auth.Claims["email"] == "a"'],
          },
        ],
      }),
      { name: "one" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "attribute", expression: 'request.Auth.Claims["email"] == "a"' },
    ]);
  });

  it("checks expression rows: empty, repeats, length and the total", () => {
    const check = (values: string[], over: Partial<BudgetFormState> = {}) =>
      validateBudgetForm(
        form({ filters: [{ ...newFilter("team"), values }], ...over }),
      );
    expect(check([])["filters.0"]).toBe("Enter an expression.");
    expect(check(["a", "a"])["filters.0"]).toBe(
      "Each expression can be added once.",
    );
    expect(check(["x".repeat(1025)])["filters.0"]).toMatch(/at most 1024/);
    expect(check(["a", "b"])).toEqual({});
    // An empty row is allowed when the limit is split by it.
    expect(
      check([], {
        splitBy: "team",
        splitExpression: 'request.Auth.Claims["groups"]',
      })["filters.0"],
    ).toBeUndefined();
    const many = (n: number) =>
      validateBudgetForm(
        form({
          filters: [
            {
              ...newFilter("project"),
              values: Array.from({ length: n }, (_, i) => `p${i}`),
            },
          ],
        }),
      ).filters;
    expect(many(20)).toBeUndefined();
    expect(many(21)).toMatch(/at most 20/);
  });

  it("takes All on its own, or specific values, and each value once", () => {
    const check = (values: string[]) =>
      validateBudgetForm(
        form({ filters: [{ ...newFilter("project"), values }] }),
      )["filters.0"];
    expect(check([ANY])).toBeUndefined();
    expect(check(["a", "b"])).toBeUndefined();
    expect(check([])).toBe("Choose one or more.");
    expect(check([ANY, "a"])).toBe("Choose All, or specific ones.");
    expect(check(["a", "a"])).toBe("Each value can be added once.");
  });

  it("offers a type to split by only while its filter is All", () => {
    const base = { amount: "1", displayName: "a" };
    const splitAgent = (filters: BudgetFormState["filters"]) =>
      validateBudgetForm(form({ ...base, filters, splitBy: "agent" })).splitBy;
    expect(splitAgent([newFilter("agent")])).toBeUndefined();
    expect(splitAgent([{ ...newFilter("agent"), values: ["p/a"] }])).toMatch(
      /filter to All/,
    );
    expect(
      splitAgent([{ ...newFilter("agent"), values: ["p/a", "p/b"] }]),
    ).toMatch(/filter to All/);
    // Several projects, split by agent: the project filter does not matter.
    expect(
      validateBudgetForm(
        form({
          ...base,
          filters: [
            { ...newFilter("project"), values: ["a", "b"] },
            newFilter("agent"),
          ],
          splitBy: "agent",
        }),
      ),
    ).toEqual({});
  });

  it("allows at most 20 filter values in total", () => {
    const values = (n: number) => Array.from({ length: n }, (_, i) => `p${i}`);
    const at = (n: number) =>
      validateBudgetForm(
        form({ filters: [{ ...newFilter("project"), values: values(n) }] }),
      ).filters;
    expect(at(20)).toBeUndefined();
    expect(at(21)).toMatch(/at most 20/);
  });

  it("points a server error on any value of a row at that row", () => {
    const rows = [
      { type: "project" as const, values: ["a", "b", "c"] },
      { type: "llmProvider" as const, values: [ANY] },
      { type: "agent" as const, values: ["a/x", "b/y"] },
    ];
    // Three project entries (0-2), then the two agents (3, 4); All providers is not sent.
    expect(sentFilterRows(rows)).toEqual([0, 0, 0, 2, 2]);
    expect(formFieldOf("spec.filters[1].id", sentFilterRows(rows))).toBe(
      "filters.0",
    );
    expect(formFieldOf("spec.filters[4].id", sentFilterRows(rows))).toBe(
      "filters.2",
    );
  });

  it("writes a single value exactly as before", () => {
    const budget = budgetFromForm(
      form({
        filters: [
          { ...newFilter("project"), values: ["online-shop"] },
          newFilter("apiKey"),
        ],
      }),
      { name: "single" },
    );
    expect(budget.spec.filters).toEqual([
      { type: "project", id: "online-shop" },
      { type: "apiKey" },
    ]);
  });

  it("groups the entries of one type as an or, and keeps each expression apart", () => {
    expect(
      groupFilterEntries([
        { type: "project", id: "a" },
        { type: "llmProvider", id: "openai" },
        { type: "project", id: "b" },
        { type: "attribute", expression: "x" },
        { type: "attribute", expression: "y" },
        { type: "agent" },
      ]),
    ).toEqual([
      [
        { type: "project", id: "a" },
        { type: "project", id: "b" },
      ],
      [{ type: "llmProvider", id: "openai" }],
      [{ type: "attribute", expression: "x" }],
      [{ type: "attribute", expression: "y" }],
      [{ type: "agent" }],
    ]);
  });

  it("sends an exemption row as one entry per value, and reads them back as one row", () => {
    const budget = budgetFromForm(
      form({
        exemptions: [
          {
            ...newExemption("agent"),
            values: ["shop/returns-bot", "claims/triage"],
          },
          { ...newExemption("project"), values: ["finance"] },
        ],
      }),
      { name: "ex" },
    );
    expect(budget.spec.exemptions).toEqual([
      { type: "agent", id: "returns-bot", project: "shop" },
      { type: "agent", id: "triage", project: "claims" },
      { type: "project", id: "finance" },
    ]);
    const back = formFromBudget(budget);
    expect(back.exemptions.map((e) => [e.type, e.values])).toEqual([
      ["agent", ["shop/returns-bot", "claims/triage"]],
      ["project", ["finance"]],
    ]);
    expect(budgetFromForm(back, { name: "ex" }).spec.exemptions).toEqual(
      budget.spec.exemptions,
    );
  });

  it("groups exemption expressions into one row per kind", () => {
    const entries = [
      {
        type: "attribute" as const,
        expression: 'request.Auth.Claims["email"] == "a"',
      },
      {
        type: "attribute" as const,
        expression: 'request.Auth.Claims["email"] == "b"',
      },
      { type: "agent" as const, id: "x", project: "p" },
      { type: "attribute" as const, expression: 'request.Headers["h"] == "1"' },
    ];
    const rows = formFromBudget(
      budgetFromForm(form({}), { name: "c" }),
    ).exemptions;
    expect(rows).toEqual([]);
    const withEntries = formFromBudget({
      ...budgetFromForm(form({}), { name: "c" }),
      spec: {
        ...budgetFromForm(form({}), { name: "c" }).spec,
        exemptions: entries,
      },
    });
    expect(withEntries.exemptions.map((e) => [e.type, e.values])).toEqual([
      [
        "user",
        [
          'request.Auth.Claims["email"] == "a"',
          'request.Auth.Claims["email"] == "b"',
        ],
      ],
      ["agent", ["p/x"]],
      ["attribute", ['request.Headers["h"] == "1"']],
    ]);
    expect(budgetFromForm(withEntries, { name: "c" }).spec.exemptions).toEqual(
      entries,
    );
  });

  it("writes a single-value exemption row exactly as before", () => {
    const budget = budgetFromForm(
      form({
        exemptions: [
          { ...newExemption("agent"), values: ["online-shop/returns-bot"] },
          { ...newExemption("project"), values: ["finance"] },
          {
            ...newExemption("team"),
            values: ["'R&D' in request.Auth.Claims[\"groups\"]"],
          },
        ],
      }),
      { name: "old" },
    );
    expect(budget.spec.exemptions).toEqual([
      { type: "agent", id: "returns-bot", project: "online-shop" },
      { type: "project", id: "finance" },
      {
        type: "attribute",
        expression: "'R&D' in request.Auth.Claims[\"groups\"]",
      },
    ]);
  });

  it("checks exemption rows: values, repeats, expressions and the total", () => {
    const check = (exemptions: BudgetFormState["exemptions"]) =>
      validateBudgetForm(form({ exemptions }));
    expect(
      check([{ ...newExemption("project"), values: [] }])["exemptions.0"],
    ).toBe("Choose one or more.");
    expect(
      check([{ ...newExemption("project"), values: ["a", "a"] }])[
        "exemptions.0"
      ],
    ).toBe("Each value can be added once.");
    expect(check([newExemption("team")])["exemptions.0"]).toBe(
      "Enter an expression.",
    );
    expect(check([{ ...newExemption("project"), values: ["a", "b"] }])).toEqual(
      {},
    );
    const many = (n: number) =>
      check([
        {
          ...newExemption("agent"),
          values: Array.from({ length: n }, (_, i) => `p/a${i}`),
        },
      ]).exemptions;
    expect(many(100)).toBeUndefined();
    expect(many(101)).toBe("Use at most 100 exemption values in total.");
  });

  it("points a server error on any value of an exemption row at that row", () => {
    const rows = [
      { ...newExemption("agent"), values: ["a/x", "a/y", "b/z"] },
      { ...newExemption("team"), values: ["e", "f"] },
      { ...newExemption("project"), values: ["f"] },
    ];
    expect(sentExemptionRows(rows)).toEqual([0, 0, 0, 1, 1, 2]);
    expect(
      formFieldOf("spec.exemptions[2].id", undefined, sentExemptionRows(rows)),
    ).toBe("exemptions.0");
    expect(
      formFieldOf(
        "spec.exemptions[3].expression",
        undefined,
        sentExemptionRows(rows),
      ),
    ).toBe("exemptions.1");
    expect(
      formFieldOf(
        "spec.exemptions[4].expression",
        undefined,
        sentExemptionRows(rows),
      ),
    ).toBe("exemptions.1");
    expect(
      formFieldOf("spec.exemptions[5].id", undefined, sentExemptionRows(rows)),
    ).toBe("exemptions.2");
    expect(
      formFieldOf("spec.exemptions[9].id", undefined, sentExemptionRows(rows)),
    ).toBe("exemptions");
  });
});
