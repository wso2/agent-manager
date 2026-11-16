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

import type {
  ShadowCapabilitySpec,
  ShadowConnectionResponse,
  ShadowProviderSpec,
} from "@agent-management-platform/types";

/** The part of the connection form these checks read. */
export interface CredentialDraft {
  values: Record<string, string | boolean>;
  keyMaps: Record<string, { id: string; key: string }[]>;
  /** Stored secrets the user chose to remove on save. */
  removed: string[];
}

/**
 * Whether a field or endpoint with this option applies, given the form's
 * checkboxes: always without one, "key" when ticked, "!key" when not.
 */
export function optionActive(
  option: string | undefined,
  values: Record<string, string | boolean>,
): boolean {
  if (!option) return true;
  if (option.startsWith("!")) return values[option.slice(1)] !== true;
  return values[option] === true;
}

/**
 * Whether a field or option holds something the scan can use: a value, a
 * stored secret that is not being removed, a key map row with a key (typed
 * or stored), a list with an item, or a ticked checkbox. A field shown only
 * by an unticked option holds nothing.
 */
export function hasValue(
  spec: ShadowProviderSpec,
  draft: CredentialDraft,
  existing: ShadowConnectionResponse | undefined,
  key: string,
): boolean {
  const f = spec.fields.find((x) => x.key === key);
  if (!f) return false;
  if (!optionActive(f.option, draft.values)) return false;
  const v = draft.values[key];
  switch (f.kind) {
    case "checkbox":
      return v === true;
    case "secret":
    case "textarea":
      return (
        String(v ?? "").trim() !== "" ||
        (!!existing?.secretHints?.[key] && !draft.removed.includes(key))
      );
    case "key_map":
      return (draft.keyMaps[key] ?? []).some(
        (r) =>
          r.id.trim() !== "" &&
          (r.key.trim() !== "" || !!existing?.secretHints?.[`${key}.${r.id.trim()}`]),
      );
    case "list":
      return String(v ?? "")
        .split(/[\n,]/)
        .some((s) => s.trim() !== "");
    default:
      return String(v ?? "").trim() !== "";
  }
}

/** The label of a field or option, for "Needs: …". */
export function needLabel(spec: ShadowProviderSpec, key: string): string {
  return spec.fields.find((f) => f.key === key)?.label ?? key;
}

export interface CapabilityStatus {
  capability: ShadowCapabilitySpec;
  available: boolean;
  /** Labels of the fields or options it still needs. */
  missing: string[];
}

/** What the scan can list with the form as it is now. */
export function evaluateCapabilities(
  spec: ShadowProviderSpec,
  draft: CredentialDraft,
  existing: ShadowConnectionResponse | undefined,
): CapabilityStatus[] {
  return (spec.capabilities ?? []).map((capability) => {
    const missing = capability.needs
      .filter((k) => !hasValue(spec, draft, existing, k))
      .map((k) => needLabel(spec, k));
    return { capability, available: missing.length === 0, missing };
  });
}

/** The platform's "one of these" message when none is set. */
export function credentialChoiceError(
  spec: ShadowProviderSpec,
  draft: CredentialDraft,
  existing: ShadowConnectionResponse | undefined,
): string | undefined {
  const choice = spec.credentialChoice;
  if (!choice) return undefined;
  return choice.fields.some((k) => hasValue(spec, draft, existing, k)) ? undefined : choice.message;
}

/**
 * How many project (OpenAI) or workspace (Anthropic) keys a connection has,
 * for the platforms list; undefined for the other platforms.
 */
export function scopedKeyCount(
  provider: string,
  secretHints: Record<string, string> | undefined,
): number | undefined {
  if (provider !== "openai" && provider !== "anthropic") return undefined;
  return Object.keys(secretHints ?? {}).filter((k) => k.startsWith("scopedKeys.")).length;
}
