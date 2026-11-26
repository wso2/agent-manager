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

// The budget form's state, and how it maps to and from the Budget kind. Kept free of
// React so the rules (handle, filters, split, validation) are tested on their own.
import {
  BUDGET_API_VERSION,
  type Budget,
  type BudgetAction,
  type BudgetExemption,
  type BudgetFilter,
  type BudgetPeriod,
} from "@agent-management-platform/types";
import { isNetworkError } from "@agent-management-platform/api-client";

/**
 * What a filter, split or exemption is about. Team and User are custom attributes with a
 * CEL expression on a token claim, offered by name as the design does (UX 3).
 */
export type ScopeKind =
  | "llmProvider"
  | "project"
  | "agent"
  | "apiKey"
  | "team"
  | "user"
  | "attribute";

/** The longest CEL expression the API takes: gateways run it on every request. */
export const MAX_EXPRESSION_LENGTH = 1024;

/** Kinds that take a CEL expression. */
export const isCelKind = (kind: ScopeKind) =>
  kind === "team" || kind === "user" || kind === "attribute";

/** The expression telling values apart when the limit is split by Team or User. */
export function identityExpression(kind: ScopeKind): string {
  if (kind === "team") return 'request.Auth.Claims["groups"]';
  if (kind === "user") return 'request.Auth.Claims["email"]';
  return "";
}

/** An example for a Team, User or custom attribute filter. */
export function expressionPlaceholder(kind: ScopeKind): string {
  if (kind === "team") return `e.g. 'R&D' in request.Auth.Claims["groups"]`;
  if (kind === "user")
    return `e.g. request.Auth.Claims["email"] == 'alice@wso2.com'`;
  return `e.g. request.Headers["x-cost-center"] == 'finance'`;
}

/** The kind a stored attribute reads as, from the claim it names. */
function kindOfExpression(expression?: string): "team" | "user" | "attribute" {
  if (expression?.includes('Claims["groups"]')) return "team";
  if (expression?.includes('Claims["email"]')) return "user";
  return "attribute";
}

/** A filter's id meaning "all of them": a project, agent or API key filter without id
 * (usage that has one, Kong's "is set"); for LLM providers, no provider filter at all. */
export const ANY = "*";

/**
 * A filter as the form edits it: which usage the budget counts. One row per type for LLM
 * providers, projects, agents and API keys, holding every value picked: a request matches
 * the row when it matches any of them (OR); different rows must all match (AND).
 */
export interface FilterRow {
  type: ScopeKind;
  /**
   * LLM provider, project, agent or API key: [ANY] for all of them, or the values picked:
   * handles, API key names, or "project/agent" for agents. Team, User and custom
   * attribute: the CEL expressions entered, any of which may match (OR). Empty until one is
   * picked.
   */
  values: string[];
}

/** Whether a row counts every LLM provider, project, agent or API key. */
export const isAllValues = (row: Pick<FilterRow, "values">): boolean =>
  row.values.length === 1 && row.values[0] === ANY;

/**
 * The expressions of one row as the one expression sent: a request matches the row when it
 * matches any of them (OR), so several are joined as "(a) || (b)"; one is sent as it is.
 */
export function joinExpressions(values: string[]): string {
  const list = values.map((v) => v.trim()).filter(Boolean);
  return list.length > 1
    ? list.map((v) => `(${v})`).join(" || ")
    : (list[0] ?? "");
}

/** The expressions a stored one was joined from: the reverse of joinExpressions. Anything
 * not shaped "(a) || (b)" is a single expression. */
export function splitExpressions(expression: string): string[] {
  const text = expression.trim();
  const groups: string[] = [];
  let depth = 0;
  let start = 0;
  let quote = "";
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) {
      if (c === "\\") i++;
      else if (c === quote) quote = "";
      continue;
    }
    if (c === "'" || c === '"') quote = c;
    else if (c === "(") {
      if (depth === 0) start = i + 1;
      depth++;
    } else if (c === ")") {
      depth--;
      if (depth < 0) return [text];
      if (depth === 0) groups.push(text.slice(start, i));
    }
  }
  const joined = groups.map((g) => `(${g})`).join(" || ");
  return depth === 0 && !quote && groups.length > 1 && joined === text
    ? groups
    : [text];
}

/**
 * The values after adding what was typed: only its ends are trimmed (a CEL expression has
 * spaces, quotes and brackets), nothing blank or already there is added, and an
 * expression above MAX_EXPRESSION_LENGTH is refused with the reason.
 */
export function addExpressions(
  values: string[],
  typed: string | string[],
): { values: string[]; error?: string } {
  const next = [...values];
  for (const text of Array.isArray(typed) ? typed : [typed]) {
    const expression = text.trim();
    if (!expression) continue;
    if (expression.length > MAX_EXPRESSION_LENGTH)
      return {
        values,
        error: `Each expression can have at most ${MAX_EXPRESSION_LENGTH} characters.`,
      };
    if (next.includes(expression))
      return { values, error: "Each expression can be added once." };
    next.push(expression);
  }
  if (joinExpressions(next).length > MAX_EXPRESSION_LENGTH)
    return {
      values,
      error: `The expressions of a row can have at most ${MAX_EXPRESSION_LENGTH} characters together.`,
    };
  return { values: next };
}

/** The most filter entries a budget may have in total (every value of a row counts). */
export const MAX_FILTER_ENTRIES = 20;

/**
 * An exemption as the form edits it: traffic the budget skips. One row per type, holding
 * every value picked ("project/agent" for agents), or, for Team, User and custom
 * attribute, the CEL expressions entered.
 */
export interface ExemptionRow {
  type: ScopeKind;
  values: string[];
}

/** The most exemption entries a budget may have in total (every value of a row counts). */
export const MAX_EXEMPTION_ENTRIES = 100;

export interface BudgetFormState {
  displayName: string;
  description: string;
  /** metadata.labels: one-word labels, as typed (blank rows are dropped when sent). */
  labels: string[];
  /** Which usage counts; all must match. Empty: all LLM usage in the organization. */
  filters: FilterRow[];
  /** A separate limit per value of this kind; "" is one shared limit. */
  splitBy: ScopeKind | "";
  /** Team, User, custom attribute split: the expression telling values apart. */
  splitExpression: string;
  exemptions: ExemptionRow[];
  amount: string;
  period: BudgetPeriod;
  action: BudgetAction;
  /** Block only, 1-100. */
  blockAt: string;
  thresholds: number[];
  channels: string[];
}

export const EMPTY_FORM: BudgetFormState = {
  displayName: "",
  description: "",
  labels: [],
  filters: [],
  splitBy: "",
  splitExpression: "",
  exemptions: [],
  amount: "",
  period: "month",
  action: "block",
  blockAt: "100",
  thresholds: [],
  channels: [],
};

/** A new filter starts as "All": every LLM provider, project, agent or API key. */
export const newFilter = (type: ScopeKind): FilterRow => ({
  type,
  values: isCelKind(type) ? [] : [ANY],
});

export const newExemption = (type: ExemptionRow["type"]): ExemptionRow => ({
  type,
  values: [],
});

/**
 * The budget's handle from its name: lowercase letters, digits and hyphens, at most 50
 * characters; a number is added while it is taken ("payments", "payments-2", ...).
 */
export function handleFor(name: string, taken: Iterable<string>): string {
  const base =
    name
      .toLowerCase()
      .normalize("NFKD")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 50)
      .replace(/-+$/, "") || "budget";
  const used = new Set(taken);
  if (!used.has(base)) return base;
  for (let n = 2; ; n++) {
    const suffix = `-${n}`;
    const candidate = `${base.slice(0, 50 - suffix.length).replace(/-+$/, "")}${suffix}`;
    if (!used.has(candidate)) return candidate;
  }
}

/** Exemption rows from a budget's entries: the entries of one type are one row, in the
 * order the types first appear; expressions are grouped by the kind they read as. */
function exemptionRowsFrom(entries: BudgetExemption[]): ExemptionRow[] {
  const rows: ExemptionRow[] = [];
  for (const e of entries) {
    const type =
      e.type === "attribute" ? kindOfExpression(e.expression) : e.type;
    const value = isCelKind(type)
      ? (e.expression ?? "")
      : type === "agent"
        ? `${e.project ?? ""}/${e.id ?? ""}`
        : (e.id ?? "");
    const row = rows.find((r) => r.type === type);
    if (!row) rows.push({ type, values: [value] } as ExemptionRow);
    else if (!row.values.includes(value)) row.values.push(value);
  }
  return rows;
}

/** The form's state for an existing budget. */
export function formFromBudget(budget: Budget): BudgetFormState {
  const { spec } = budget;
  const split = spec.limit.splitBy;
  const splitBy: ScopeKind | "" = split
    ? split.type === "attribute"
      ? kindOfExpression(split.expression)
      : split.type
    : "";
  // Several entries of one type are one row with all their values; expressions are
  // grouped by the kind they read as (Team, User or custom attribute).
  const filters: FilterRow[] = [];
  for (const f of spec.filters ?? []) {
    const type =
      f.type === "attribute" ? kindOfExpression(f.expression) : f.type;
    if (isCelKind(type)) {
      // One filter is one row of expressions (any of them matches); Team and User have one
      // row each, and every custom attribute filter is a row of its own.
      const values = splitExpressions(f.expression ?? "");
      const row =
        type === "attribute" ? undefined : filters.find((r) => r.type === type);
      if (!row) filters.push({ type, values });
      else
        for (const v of values) if (!row.values.includes(v)) row.values.push(v);
      continue;
    }
    const value =
      type === "agent" && f.id ? `${f.project ?? ""}/${f.id}` : (f.id ?? ANY);
    const row = filters.find((r) => r.type === type);
    if (!row) filters.push({ type, values: [value] });
    else if (!row.values.includes(value)) row.values.push(value);
  }
  // The limit is split by one of the filters: a split with no filter of its type (all
  // LLM providers, or a split alone) shows that filter as "All".
  if (splitBy && !filters.some((f) => f.type === splitBy)) {
    if (splitBy === "llmProvider") filters.unshift(newFilter(splitBy));
    else filters.push(newFilter(splitBy));
  }
  return {
    displayName: spec.displayName ?? budget.metadata.name,
    description: spec.description ?? "",
    labels: budget.metadata.labels ?? [],
    filters,
    splitBy,
    splitExpression: split?.expression ?? "",
    exemptions: exemptionRowsFrom(spec.exemptions ?? []),
    amount: spec.limit.amount,
    period: spec.limit.period,
    action: spec.limit.action,
    blockAt: String(spec.limit.blockAt ?? 100),
    thresholds: spec.warnings?.thresholds ?? [],
    channels: spec.warnings?.channels ?? [],
  };
}

/**
 * The filter entries a row is sent as: one per value picked (repeating the type is how
 * the Budget kind says "or"); a Team, User or custom attribute row is one entry per
 * expression, or none when it has none (it is there to split the limit by); All LLM
 * providers is no entry (every request has a provider); All projects, agents or API keys
 * is the type without an id.
 */
function entriesOf(r: FilterRow): BudgetFilter[] {
  if (isCelKind(r.type)) {
    const expression = joinExpressions(r.values);
    return expression ? [{ type: "attribute", expression }] : [];
  }
  if (r.type === "llmProvider" && isAllValues(r)) return [];
  return r.values.map((v): BudgetFilter => {
    if (v === ANY) return { type: r.type as BudgetFilter["type"] };
    if (r.type === "agent") {
      const slash = v.indexOf("/");
      return {
        type: "agent",
        project: slash < 0 ? "" : v.slice(0, slash),
        id: slash < 0 ? v : v.slice(slash + 1),
      };
    }
    return { type: r.type as BudgetFilter["type"], id: v };
  });
}

/** A filter as the Budget kind stores it. */
export type FilterEntry = {
  type: string;
  id?: string;
  project?: string;
  expression?: string;
};

/**
 * The entries of a budget's filters grouped as they are read: several entries of one LLM
 * provider, project, agent or API key type are one group ("or"); a custom attribute stays
 * on its own, as its expressions must all match. Groups keep the order their type first
 * appears in; the groups are joined by "and".
 */
export function groupFilterEntries(filters: FilterEntry[]): FilterEntry[][] {
  const groups: FilterEntry[][] = [];
  for (const f of filters) {
    const group =
      f.type === "attribute"
        ? undefined
        : groups.find((g) => g[0].type === f.type);
    if (group) group.push(f);
    else groups.push([f]);
  }
  return groups;
}

/** The form row of each sent filter, by its index in the request's `filters`. */
export function sentFilterRows(rows: FilterRow[]): number[] {
  return rows.flatMap((r, i) => entriesOf(r).map(() => i));
}

function filtersFor(rows: FilterRow[]): BudgetFilter[] {
  return rows.flatMap(entriesOf);
}

/** The entries a row is sent as: one per value picked, or its one expression. */
function exemptionEntriesOf(r: ExemptionRow): BudgetExemption[] {
  if (isCelKind(r.type))
    return r.values
      .map((v) => v.trim())
      .filter(Boolean)
      .map(
        (expression): BudgetExemption => ({ type: "attribute", expression }),
      );
  return r.values.map((v): BudgetExemption => {
    if (r.type === "agent") {
      const slash = v.indexOf("/");
      return {
        type: "agent",
        id: slash < 0 ? v : v.slice(slash + 1),
        project: slash < 0 ? "" : v.slice(0, slash),
      };
    }
    return { type: r.type as BudgetExemption["type"], id: v };
  });
}

/** The form row of each sent exemption, by its index in the request's `exemptions`. */
export function sentExemptionRows(rows: ExemptionRow[]): number[] {
  return rows.flatMap((r, i) => exemptionEntriesOf(r).map(() => i));
}

function exemptionsFor(rows: ExemptionRow[]): BudgetExemption[] {
  return rows.flatMap(exemptionEntriesOf);
}

/**
 * The Budget kind to send. A new budget is named `name`; an edit keeps the existing
 * metadata and status.
 */
export function budgetFromForm(
  form: BudgetFormState,
  opts: { name: string; existing?: Budget },
): Budget {
  const words = form.labels.map((l) => l.trim()).filter(Boolean);
  const labels = words.length ? words : undefined;
  // Labels can change on an edit; the name and annotations stay.
  const base = opts.existing?.metadata ?? { name: opts.name };
  const metadata = {
    name: base.name,
    ...(labels ? { labels } : {}),
    ...(base.annotations ? { annotations: base.annotations } : {}),
  };
  const thresholds = [...new Set(form.thresholds)].sort((a, b) => a - b);
  const splitBy = form.splitBy
    ? isCelKind(form.splitBy)
      ? { type: "attribute" as const, expression: form.splitExpression.trim() }
      : { type: form.splitBy as BudgetFilter["type"] }
    : undefined;
  return {
    apiVersion: BUDGET_API_VERSION,
    kind: "Budget",
    metadata,
    spec: {
      displayName: form.displayName.trim(),
      ...(form.description.trim()
        ? { description: form.description.trim() }
        : {}),
      ...(opts.existing?.spec.status
        ? { status: opts.existing.spec.status }
        : {}),
      limit: {
        amount: Number(form.amount).toFixed(2),
        period: form.period,
        action: form.action,
        ...(form.action === "block" ? { blockAt: Number(form.blockAt) } : {}),
        ...(splitBy ? { splitBy } : {}),
      },
      filters: filtersFor(form.filters),
      exemptions: exemptionsFor(form.exemptions),
      ...(thresholds.length
        ? { warnings: { thresholds, channels: form.channels } }
        : {}),
    },
  };
}

/**
 * Which kinds an exemption may be: any of them. What is exempted is up to the person who
 * writes the budget, so nothing is held back for being above or below what it counts.
 */
export function exemptionKinds(): ExemptionRow["type"][] {
  return [
    "llmProvider",
    "project",
    "agent",
    "apiKey",
    "team",
    "user",
    "attribute",
  ];
}

/** What a budget started from an entity's page is about. */
export interface BudgetStart {
  /** A project's own budget. */
  project?: string;
  /** An agent, as project/agent. */
  agent?: string;
  /** One LLM provider, by handle. */
  provider?: string;
}

/** The form for a budget started from an entity's page (UX 2: the entity is filled in). */
export function formFor(start: BudgetStart): BudgetFormState {
  const filters: FilterRow[] = [];
  if (start.provider)
    filters.push({ ...newFilter("llmProvider"), values: [start.provider] });
  if (start.project)
    filters.push({ ...newFilter("project"), values: [start.project] });
  if (start.agent) {
    const [project = "", id = ""] = start.agent.split("/");
    filters.push({ ...newFilter("agent"), values: [`${project}/${id}`] });
  }
  return { ...EMPTY_FORM, filters };
}

/** The start in a URL's query, as "Advanced create" passes it to the full form. */
export function startFromQuery(params: URLSearchParams): BudgetStart {
  return {
    project: params.get("project") ?? undefined,
    agent: params.get("agent") ?? undefined,
    provider: params.get("provider") ?? undefined,
  };
}

export function startQuery(start: BudgetStart): string {
  const q = new URLSearchParams();
  if (start.project) q.set("project", start.project);
  if (start.agent) q.set("agent", start.agent);
  if (start.provider) q.set("provider", start.provider);
  const text = q.toString();
  return text ? `?${text}` : "";
}

/** The amounts a budget takes: above zero, at most 12 whole digits and 2 decimals. Leading
 * zeros are allowed here, since the amount is sent normalized ("0100" becomes "100.00"). */
export const AMOUNT_PATTERN =
  /^0*([1-9]\d{0,11}(\.\d{1,2})?|0?\.(\d[1-9]|[1-9]\d?))$/;

export type BudgetFormErrors = Partial<Record<string, string>>;

/**
 * The form field a server error names (`additionalData.field`), so its message shows
 * next to the field. Errors about the filter or exemption lists as a whole, or a field
 * the form has no input for, come back as "filters", "exemptions" or "form".
 */
export function formFieldOf(
  serverField: string | undefined,
  filterRows?: number[],
  exemptionRows?: number[],
): string {
  const f = (serverField ?? "").replace(/^spec\./, "");
  const exemption = /^exemptions\[(\d+)\]/.exec(f);
  if (exemption) {
    // Several entries are one form row; map back as the filters do.
    const row = exemptionRows
      ? exemptionRows[Number(exemption[1])]
      : Number(exemption[1]);
    return row === undefined ? "exemptions" : `exemptions.${row}`;
  }
  if (f.startsWith("exemptions")) return "exemptions";
  const filter = /^filters\[(\d+)\]/.exec(f);
  if (filter) {
    // Rows the form does not send shift the request's indexes; map back to the form row.
    const row = filterRows ? filterRows[Number(filter[1])] : Number(filter[1]);
    return row === undefined ? "filters" : `filters.${row}`;
  }
  if (f.startsWith("filters")) return "filters";
  if (f.startsWith("limit.splitBy")) return "splitBy";
  if (f.startsWith("warnings.thresholds")) return "thresholds";
  if (f.startsWith("warnings.channels")) return "channels";
  if (f === "limit.amount") return "amount";
  if (f === "limit.blockAt") return "blockAt";
  if (f === "displayName" || f === "metadata.name") return "displayName";
  if (f === "description") return "description";
  if (f.startsWith("metadata.labels")) return "labels";
  return "form";
}

/** The message and field of a failed save, from the API client's error. */
export function serverFieldError(
  error: unknown,
  filterRows?: number[],
  exemptionRows?: number[],
): {
  field: string;
  message: string;
} {
  if (isNetworkError(error)) {
    return {
      field: "form",
      message: "Couldn't reach the server. Try again.",
    };
  }
  const body = (
    error as {
      body?: { message?: string; additionalData?: { field?: string } };
    }
  )?.body;
  const message =
    body?.message ??
    (error as { message?: string })?.message ??
    "The budget couldn't be saved.";
  return {
    field: formFieldOf(body?.additionalData?.field, filterRows, exemptionRows),
    message,
  };
}

/** "agent", "API key": what the limit is split by, in a sentence. */
const SPLIT_NOUN: Record<ScopeKind, string> = {
  llmProvider: "LLM provider",
  project: "project",
  agent: "agent",
  apiKey: "API key",
  team: "team",
  user: "user",
  attribute: "attribute value",
};

/** A label: one word of 1-63 letters, digits, '.', '_' or '-', starting and ending with a
 * letter or digit (the server's rule). */
export const LABEL_PATTERN = /^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$/;

/** The most labels a budget may have. */
export const MAX_LABELS = 10;

/** What is wrong with a row's values: none picked, a repeat, or a too long expression. */
function rowError(
  type: ScopeKind,
  values: string[],
  opts: { emptyOk?: boolean } = {},
): string | undefined {
  const cel = isCelKind(type);
  const picked = cel ? values.map((v) => v.trim()).filter(Boolean) : values;
  if (!picked.length) {
    if (opts.emptyOk) return undefined;
    return cel ? "Enter an expression." : "Choose one or more.";
  }
  if (!cel && picked.length > 1 && picked.includes(ANY))
    return "Choose All, or specific ones.";
  if (new Set(picked).size !== picked.length)
    return cel
      ? "Each expression can be added once."
      : "Each value can be added once.";
  if (picked.some((v) => cel && v.length > MAX_EXPRESSION_LENGTH))
    return `Each expression can have at most ${MAX_EXPRESSION_LENGTH} characters.`;
  return undefined;
}

/** What is wrong with the form, by field; empty when it can be saved. Mirrors the
 * server's rules so most mistakes show before saving. */
export function validateBudgetForm(form: BudgetFormState): BudgetFormErrors {
  const errors: BudgetFormErrors = {};
  if (!form.displayName.trim()) errors.displayName = "Enter a name.";
  if (!form.amount.trim()) {
    errors.amount = "Enter an amount.";
  } else if (
    !AMOUNT_PATTERN.test(form.amount.trim()) ||
    Number(form.amount) <= 0
  ) {
    errors.amount = "Enter an amount above 0, up to 2 decimals.";
  }
  if (form.action === "block") {
    const blockAt = Number(form.blockAt);
    if (!Number.isInteger(blockAt) || blockAt < 1 || blockAt > 100) {
      errors.blockAt = "Enter a whole percentage from 1 to 100.";
    }
  }
  form.filters.forEach((f, i) => {
    const message = rowError(f.type, f.values, {
      // An empty expression row is there to split the limit by it.
      emptyOk: isCelKind(f.type) && form.splitBy === f.type,
    });
    if (message) errors[`filters.${i}`] = message;
  });
  if (filtersFor(form.filters).length > MAX_FILTER_ENTRIES) {
    errors.filters = `Use at most ${MAX_FILTER_ENTRIES} filter values in total.`;
  }
  if (form.splitBy) {
    const rows = form.filters.filter((f) => f.type === form.splitBy);
    if (!rows.length) {
      errors.splitBy = "Add this type under Applies to first.";
    } else if (!isCelKind(form.splitBy) && rows.some((f) => !isAllValues(f))) {
      errors.splitBy = `Set the ${SPLIT_NOUN[form.splitBy]} filter to All to split by it.`;
    } else if (isCelKind(form.splitBy) && !form.splitExpression.trim()) {
      errors.splitBy = "Enter an identity expression.";
    }
  }
  form.exemptions.forEach((e, i) => {
    const message = rowError(e.type, e.values);
    if (message) errors[`exemptions.${i}`] = message;
  });
  if (exemptionsFor(form.exemptions).length > MAX_EXEMPTION_ENTRIES) {
    errors.exemptions = `Use at most ${MAX_EXEMPTION_ENTRIES} exemption values in total.`;
  }
  // Labels: one word each (the server's rule), no repeats, at most 10.
  const words = form.labels.map((l) => l.trim()).filter(Boolean);
  if (words.length > MAX_LABELS) {
    errors.labels = `Add at most ${MAX_LABELS} labels.`;
  } else if (words.some((w) => !LABEL_PATTERN.test(w))) {
    errors.labels = "Use one word: letters, digits, '.', '_' or '-'.";
  } else if (new Set(words).size !== words.length) {
    errors.labels = "Each label can be added once.";
  }
  // The server's rules: 1-1000% (above 100 warns after the limit is passed), at most 10.
  if (form.thresholds.some((t) => !Number.isInteger(t) || t < 1 || t > 1000)) {
    errors.thresholds = "Enter whole percentages between 1 and 1000.";
  } else if (form.thresholds.length > 10) {
    errors.thresholds = "Add at most 10 warning thresholds.";
  }
  if (form.channels.length && !form.thresholds.length) {
    errors.thresholds = "Add a threshold, or remove the channels.";
  }
  if (form.thresholds.length && !form.channels.length) {
    errors.channels = "Choose where warnings go (1 to 20 channels).";
  }
  if (form.channels.length > 20)
    errors.channels = "Choose at most 20 channels.";
  return errors;
}

/**
 * Another budget already uses this display name (ignoring case and spaces at either end):
 * names must tell budgets apart on every page. ownName is the budget being edited.
 */
export function duplicateNameError(
  displayName: string,
  budgets: { metadata: { name: string }; spec: { displayName?: string } }[],
  ownName?: string,
): string | undefined {
  const name = displayName.trim().toLowerCase();
  if (!name) return undefined;
  const clash = budgets.find(
    (b) =>
      b.metadata.name !== ownName &&
      (b.spec.displayName ?? b.metadata.name).trim().toLowerCase() === name,
  );
  return clash
    ? `A budget named ${displayName.trim()} already exists.`
    : undefined;
}

/**
 * The budget asked for does not exist: a 404 (the API client's GET errors carry only the
 * status), or the server's BUDGET_NOT_FOUND body.
 */
export function isBudgetNotFound(error: unknown): boolean {
  const e = error as { code?: unknown; status?: unknown } | null;
  return e?.status === 404 || e?.code === "BUDGET_NOT_FOUND";
}

/** One-word labels as the shared label chips take them: each word a key, no value. */
export const wordLabels = (labels: string[]): Record<string, string> =>
  Object.fromEntries(labels.map((l) => [l, ""]));
