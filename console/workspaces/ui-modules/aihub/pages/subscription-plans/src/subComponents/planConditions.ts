/*
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

import type { SubscriptionPlanCondition } from "@agent-management-platform/types";

/** One condition of a plan, as the subscription plans API takes and returns it. */
export type PlanCondition = SubscriptionPlanCondition;
export type ConditionType = PlanCondition["type"];
export type BuiltInType = Exclude<ConditionType, "custom">;
export type TimeUnit = "MINUTE" | "HOUR" | "DAY" | "MONTH";
export type OnExceed = "reject" | "allow";
export type TokenType = "total" | "prompt" | "completion";

/** The window of a built-in condition, in the shape the API takes. */
export type ConditionWindow = { count: number; timeAmount: number; timeUnit: TimeUnit };

/** The params of a built-in condition, in the shape the API takes. */
export type ConditionLimits = {
  limits: ConditionWindow[];
  onExceed: OnExceed;
  tokenType?: TokenType;
};

/** The id each built-in type fixes, so the policy that enforces it finds it in every plan. */
export const BUILT_IN_IDS: Record<BuiltInType, string> = {
  "request-count": "requests",
  "token-count": "tokens",
  "tool-call-count": "tool-calls",
};

/** The built-in types, in the order they are offered. */
export const BUILT_IN_TYPES: BuiltInType[] = ["request-count", "tool-call-count", "token-count"];

export const TIME_UNITS: TimeUnit[] = ["MINUTE", "HOUR", "DAY", "MONTH"];
export const TOKEN_TYPES: TokenType[] = ["total", "prompt", "completion"];

/** What a plan can hold; the subscription plans API refuses more (plan_conditions.go). */
export const MAX_CONDITIONS = 20;
export const MAX_WINDOWS = 10;
export const MAX_CUSTOM_PARAMS_BYTES = 4096;
export const MAX_CUSTOM_ID_CHARS = 63;
export const MAX_DESCRIPTION_CHARS = 1023;
/** The largest count or window length a number here holds exactly; the plans API takes up to its
 * 64-bit integer range, more than a JavaScript number can carry without rounding. */
export const MAX_WHOLE_NUMBER = Number.MAX_SAFE_INTEGER;
const CUSTOM_ID = /^[a-z][a-z0-9-]{0,62}$/;

/**
 * The params key under which a custom condition names the policy that enforces it:
 * `{"policy": {"name": "region-guard", "version": "v1"}}`. A reserved key until the API gets a
 * field of its own for it; the key-value editor never shows or accepts it.
 */
export const CUSTOM_POLICY_KEY = "policy";
const POLICY_NAME = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$/;
const POLICY_VERSION = /^v?\d+(\.\d+){0,2}$/;

/** Own keys only, so a custom type or ID such as `constructor` is not taken for a built-in one. */
export const isBuiltInType = (type: string): type is BuiltInType =>
  Object.prototype.hasOwnProperty.call(BUILT_IN_IDS, type);

/** The policy a custom condition names, if it names one, with its version as `v<major>`. */
export const customPolicyOf = (condition: {
  type: string;
  params?: unknown;
}): { name: string; version: string } | null => {
  if (isBuiltInType(condition.type)) return null;
  const policy = (condition.params as Record<string, unknown> | undefined)?.[CUSTOM_POLICY_KEY] as
    | { name?: unknown; version?: unknown }
    | undefined;
  if (!policy || typeof policy !== "object" || typeof policy.name !== "string" || !policy.name) {
    return null;
  }
  const version = typeof policy.version === "string" ? policy.version : "v1";
  const major = version.match(/^v?(\d+)/)?.[1];
  return { name: policy.name, version: major ? `v${major}` : version };
};

/** A window being edited. Numeric fields stay strings while the user types. */
export type WindowDraft = { key: number; count: string; timeAmount: string; timeUnit: TimeUnit };

/** A key-value pair of a custom condition. `saved` is the name and value it was loaded with, each
 * sent back as it was while untouched (so a saved "00123" stays a string, and " region " keeps its
 * spaces). */
export type PairDraft = {
  key: number;
  name: string;
  value: string;
  saved?: { name: string; text: string; value: unknown };
};

/** A condition being edited. A built-in one uses `windows`, `onExceed` and (token-count)
 * `tokenType`; a custom one uses `id`, `pairs` and, optionally, the policy that enforces it. */
export type ConditionDraft = {
  key: number;
  type: ConditionType;
  id: string;
  windows: WindowDraft[];
  onExceed: OnExceed;
  tokenType: TokenType;
  pairs: PairDraft[];
  /** Custom only: the policy that enforces it, sent as `params.policy`; empty when none. */
  policyName: string;
  policyVersion: string;
  /** Custom only: the saved `params.policy`, with the name and version shown for it. While
   * those two fields are left as shown, the saved value is sent back as it was, even a shape
   * this editor cannot show (a plain string, or more fields than name and version). */
  savedPolicy?: { value: unknown; name: string; version: string };
};

let nextKey = 0;
/** Stable React keys for rows that carry no id of their own until the plan is saved. */
export const newKey = () => nextKey++;

export const newWindow = (window?: Partial<ConditionWindow>): WindowDraft => ({
  count: window?.count != null ? String(window.count) : "",
  key: newKey(),
  timeAmount: String(window?.timeAmount ?? 1),
  timeUnit: window?.timeUnit && TIME_UNITS.includes(window.timeUnit) ? window.timeUnit : "MINUTE",
});

const valueText = (value: unknown) => (typeof value === "string" ? value : JSON.stringify(value));

export const newPair = (name = "", value?: unknown): PairDraft => {
  if (value === undefined) return { key: newKey(), name, value: "" };
  const text = valueText(value);
  return { key: newKey(), name, saved: { name, text, value }, value: text };
};

/** A new condition of the given type: a built-in one starts with one empty window, a custom one
 * with one empty pair. */
export const newCondition = (type: ConditionType): ConditionDraft => ({
  id: isBuiltInType(type) ? BUILT_IN_IDS[type] : "",
  key: newKey(),
  onExceed: "reject",
  pairs: type === "custom" ? [newPair()] : [],
  policyName: "",
  policyVersion: "",
  tokenType: "total",
  type,
  windows: type === "custom" ? [] : [newWindow()],
});

/** The editor's drafts of a saved plan's conditions. A type this console does not know is shown as
 * a custom condition. */
export const draftsFromConditions = (conditions: PlanCondition[] | undefined): ConditionDraft[] =>
  (conditions ?? []).map((condition) => {
    const params = (condition.params ?? {}) as Record<string, unknown>;
    if (!isBuiltInType(condition.type)) {
      const pairs = Object.entries(params)
        .filter(([name]) => name !== CUSTOM_POLICY_KEY)
        .map(([name, value]) => newPair(name, value));
      const saved = params[CUSTOM_POLICY_KEY];
      const policy =
        saved && typeof saved === "object"
          ? (saved as { name?: unknown; version?: unknown })
          : undefined;
      const policyName = typeof policy?.name === "string" ? policy.name : "";
      const policyVersion = typeof policy?.version === "string" ? policy.version : "";
      return {
        ...newCondition("custom"),
        id: condition.id ?? "",
        pairs: pairs.length > 0 ? pairs : [newPair()],
        policyName,
        policyVersion,
        savedPolicy:
          saved === undefined
            ? undefined
            : { value: saved, name: policyName, version: policyVersion },
      };
    }
    const limits = (params as Partial<ConditionLimits>).limits ?? [];
    return {
      ...newCondition(condition.type),
      onExceed: params.onExceed === "allow" ? "allow" : "reject",
      tokenType: TOKEN_TYPES.includes(params.tokenType as TokenType)
        ? (params.tokenType as TokenType)
        : "total",
      windows: limits.length > 0 ? limits.map((limit) => newWindow(limit)) : [newWindow()],
    };
  });

/**
 * Whether `literal`, a number as written (decimal, optionally with an exponent), is held exactly by
 * a JavaScript number: finite, not rounded to 0 (underflow), an integer within the safe range, and
 * at most 15 significant digits. Otherwise it is kept as text, never silently changed.
 */
export const exactNumber = (literal: string): boolean => {
  const value = Number(literal);
  if (!Number.isFinite(value)) return false;
  const mantissa = literal.replace(/^-/, "").replace(/[eE].*$/, "");
  if (value === 0 && /[1-9]/.test(mantissa)) return false;
  if (Number.isInteger(value) && !Number.isSafeInteger(value)) return false;
  const digits = mantissa.replace(".", "").replace(/^0+/, "");
  return digits.length <= 15;
};

/**
 * Whether every number literal in `json` (outside strings) is exact (see exactNumber). Checked on
 * the text before parsing, so no engine's rounded value is trusted.
 */
export const exactJsonNumbers = (json: string): boolean => {
  let inString = false;
  for (let i = 0; i < json.length; i += 1) {
    const ch = json[i];
    if (inString) {
      if (ch === "\\") i += 1;
      else if (ch === '"') inString = false;
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === "-" || (ch >= "0" && ch <= "9")) {
      const match = /^-?\d+(\.\d+)?([eE][+-]?\d+)?/.exec(json.slice(i));
      if (!match) continue;
      if (!exactNumber(match[0])) return false;
      i += match[0].length - 1;
    }
  }
  return true;
};

/** A typed value: a number (when exact), true or false, a JSON object or list, or else the text as
 * it is. A JSON value with a number it can't hold exactly is kept as text too. */
export const parseValue = (text: string): unknown => {
  if (/^-?\d+(\.\d+)?$/.test(text)) {
    return exactNumber(text) ? Number(text) : text;
  }
  if (text === "true" || text === "false") return text === "true";
  if (/^[[{]/.test(text)) {
    // A number the JSON can't keep exactly keeps the whole value as text.
    if (!exactJsonNumbers(text)) return text;
    try {
      return JSON.parse(text) as unknown;
    } catch {
      // Not JSON, or a number it can't hold exactly: kept as text.
    }
  }
  return text;
};

/** The editor's messages, in the console's language (it has no translations). */
const messages = {
  amountInvalid: () => "Enter a whole number of at least 1.",
  countInvalid: () => "Enter a whole number of at least 1.",
  customIdBuiltIn: (id: string) => `${id} is the name of a built-in condition; choose another.`,
  customIdInvalid: () =>
    "Use lowercase letters, digits and hyphens, starting with a letter (at most 63 characters).",
  customIdTaken: (id: string) => `Another condition already has the name ${id}.`,
  numberTooLarge: (max: number) => `Enter a whole number of at most ${max.toLocaleString()}.`,
  noWindows: () => "Add at least one limit, or remove the condition.",
  pairKeyMissing: () => "Each value needs a key.",
  pairKeyReserved: (name: string) =>
    `The key ${name} is reserved for the custom policy; use another key.`,
  policyNameInvalid: () => "Enter the policy name, for example region-guard.",
  policyVersionInvalid: () => "Enter a version such as v1.",
  pairKeyTaken: (name: string) => `The key ${name} is used twice.`,
  paramsTooLarge: (max: number) =>
    `The values must be at most ${max.toLocaleString()} bytes in all.`,
  tooManyConditions: (max: number) => `A plan can have at most ${max} conditions.`,
  tooManyWindows: (max: number) => `A condition can have at most ${max} limits.`,
  windowTaken: () =>
    "Another limit already covers this window; each limit needs a window of its own length.",
};

export type FieldError = { message: string };

/** The keys a field's error is filed under, so the editor finds it and knows when it was
 * touched. */
export const fieldKeys = {
  amount: (condition: number, window: number) => `${condition}.window.${window}.amount`,
  conditions: () => "conditions",
  count: (condition: number, window: number) => `${condition}.window.${window}.count`,
  customId: (condition: number) => `${condition}.id`,
  pairName: (condition: number, pair: number) => `${condition}.pair.${pair}.name`,
  policyName: (condition: number) => `${condition}.policy.name`,
  policyVersion: (condition: number) => `${condition}.policy.version`,
  params: (condition: number) => `${condition}.params`,
  /** A window whose length another window already has: reported on its unit. */
  unit: (condition: number, window: number) => `${condition}.window.${window}.unit`,
  windows: (condition: number) => `${condition}.windows`,
};

export type ConditionsCheck = {
  /** Every problem, by field key. Empty when the conditions can be sent. */
  errors: Map<string, FieldError>;
  /** The conditions which have a problem, by their draft key. */
  conditionsWithErrors: Set<number>;
  /** The conditions as the API takes them, in order; `null` while there is a problem. */
  body: PlanCondition[] | null;
};

/** The length of a window, so that 60 MINUTE and 1 HOUR are the same; a MONTH is its own length.
 * A length in minutes past the exact range can't equal another window: kept apart, as the
 * subscription plans API does (windowKey). */
const windowLength = (amount: number, unit: TimeUnit) => {
  if (unit === "MONTH") return `M${amount}`;
  const minutes = amount * { DAY: 1440, HOUR: 60, MINUTE: 1 }[unit];
  return Number.isSafeInteger(minutes) ? `m${minutes}` : `${unit}${amount}`;
};

const isWholeNumber = (text: string) => /^\d+$/.test(text.trim());

/** A whole number as typed: its value, or why it can't be sent. `NaN` when it is not a whole
 * number, `Infinity` when it is past what a number holds exactly (rather than rounding it). */
const wholeNumber = (text: string): number => {
  if (!isWholeNumber(text)) return NaN;
  const value = Number(text.trim());
  return Number.isSafeInteger(value) ? value : Infinity;
};

/** Checks the conditions as the subscription plans API does, and builds what is sent. */
export const checkConditions = (drafts: ConditionDraft[]): ConditionsCheck => {
  const errors = new Map<string, FieldError>();
  const conditionsWithErrors = new Set<number>();
  const fail = (condition: number, key: string, error: FieldError) => {
    if (!errors.has(key)) errors.set(key, error);
    conditionsWithErrors.add(condition);
  };
  if (drafts.length > MAX_CONDITIONS) {
    errors.set(fieldKeys.conditions(), { message: messages.tooManyConditions(MAX_CONDITIONS) });
  }

  const usedIds = new Map<string, number>();
  for (const draft of drafts) {
    if (draft.type !== "custom") usedIds.set(BUILT_IN_IDS[draft.type as BuiltInType], draft.key);
  }

  const body: PlanCondition[] = drafts.map((draft) => {
    if (draft.type === "custom") {
      const id = draft.id.trim();
      if (!CUSTOM_ID.test(id)) {
        fail(draft.key, fieldKeys.customId(draft.key), { message: messages.customIdInvalid() });
      } else if (Object.values(BUILT_IN_IDS).includes(id)) {
        fail(draft.key, fieldKeys.customId(draft.key), { message: messages.customIdBuiltIn(id) });
      } else if (usedIds.has(id) && usedIds.get(id) !== draft.key) {
        fail(draft.key, fieldKeys.customId(draft.key), { message: messages.customIdTaken(id) });
      } else {
        usedIds.set(id, draft.key);
      }
      // No prototype, so a key such as `__proto__` is kept as a key of its own and sent.
      const params = Object.create(null) as Record<string, unknown>;
      for (const pair of draft.pairs) {
        // A saved name left untouched is sent as it was, spaces too; a typed one is trimmed.
        const name = pair.saved && pair.saved.name === pair.name ? pair.name : pair.name.trim();
        const text = pair.value.trim();
        // A row left empty is not a value.
        if (!name && !text) continue;
        if (!name) {
          fail(draft.key, fieldKeys.pairName(draft.key, pair.key), {
            message: messages.pairKeyMissing(),
          });
          continue;
        }
        if (name === CUSTOM_POLICY_KEY) {
          fail(draft.key, fieldKeys.pairName(draft.key, pair.key), {
            message: messages.pairKeyReserved(name),
          });
          continue;
        }
        if (Object.prototype.hasOwnProperty.call(params, name)) {
          fail(draft.key, fieldKeys.pairName(draft.key, pair.key), {
            message: messages.pairKeyTaken(name),
          });
          continue;
        }
        // The text as typed, spaces too: a saved " true " left untouched stays that string.
        params[name] =
          pair.saved && pair.saved.text === pair.value ? pair.saved.value : parseValue(text);
      }
      // The optional enforcing policy, under its reserved key; left out when not given.
      const policyName = draft.policyName.trim();
      const policyVersion = draft.policyVersion.trim();
      const savedPolicy = draft.savedPolicy;
      if (
        savedPolicy &&
        policyName === savedPolicy.name.trim() &&
        policyVersion === savedPolicy.version.trim()
      ) {
        // Untouched: the saved policy goes back as it was.
        params[CUSTOM_POLICY_KEY] = savedPolicy.value;
      } else if (policyName || policyVersion) {
        if (!POLICY_NAME.test(policyName)) {
          fail(draft.key, fieldKeys.policyName(draft.key), {
            message: messages.policyNameInvalid(),
          });
        }
        if (policyVersion && !POLICY_VERSION.test(policyVersion)) {
          fail(draft.key, fieldKeys.policyVersion(draft.key), {
            message: messages.policyVersionInvalid(),
          });
        }
        const version = policyVersion || "v1";
        params[CUSTOM_POLICY_KEY] = {
          name: policyName,
          version: version.startsWith("v") ? version : `v${version}`,
        };
      }
      if (new TextEncoder().encode(JSON.stringify(params)).length > MAX_CUSTOM_PARAMS_BYTES) {
        fail(draft.key, fieldKeys.params(draft.key), {
          message: messages.paramsTooLarge(MAX_CUSTOM_PARAMS_BYTES),
        });
      }
      return { id, params, type: "custom" };
    }

    const type = draft.type as BuiltInType;
    if (draft.windows.length === 0) {
      fail(draft.key, fieldKeys.windows(draft.key), { message: messages.noWindows() });
    } else if (draft.windows.length > MAX_WINDOWS) {
      fail(draft.key, fieldKeys.windows(draft.key), {
        message: messages.tooManyWindows(MAX_WINDOWS),
      });
    }
    const seen = new Set<string>();
    const limits: ConditionWindow[] = draft.windows.map((window) => {
      const count = wholeNumber(window.count);
      const amountText = window.timeAmount.trim();
      const timeAmount = amountText === "" ? 1 : wholeNumber(amountText);
      const tooLarge: FieldError = { message: messages.numberTooLarge(MAX_WHOLE_NUMBER) };
      if (count === Infinity) {
        fail(draft.key, fieldKeys.count(draft.key, window.key), tooLarge);
      } else if (!(count >= 1)) {
        fail(draft.key, fieldKeys.count(draft.key, window.key), {
          message: messages.countInvalid(),
        });
      }
      if (timeAmount === Infinity) {
        fail(draft.key, fieldKeys.amount(draft.key, window.key), tooLarge);
      } else if (!(timeAmount >= 1)) {
        fail(draft.key, fieldKeys.amount(draft.key, window.key), {
          message: messages.amountInvalid(),
        });
      } else {
        const length = windowLength(timeAmount, window.timeUnit);
        if (seen.has(length)) {
          fail(draft.key, fieldKeys.unit(draft.key, window.key), {
            message: messages.windowTaken(),
          });
        }
        seen.add(length);
      }
      return { count, timeAmount, timeUnit: window.timeUnit };
    });
    const params: ConditionLimits = { limits, onExceed: draft.onExceed };
    if (type === "token-count") params.tokenType = draft.tokenType;
    return { id: BUILT_IN_IDS[type], params, type };
  });

  return { body: errors.size === 0 ? body : null, conditionsWithErrors, errors };
};
