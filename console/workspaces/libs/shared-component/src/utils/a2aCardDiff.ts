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
  asObject,
  asString,
  isObject,
  objectsIn,
  type JsonObject,
} from "./a2aCardReaders";

export interface A2ACardDiff {
  fields: string[];
  skills: { added: string[]; removed: string[]; changed: string[] };
}

function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical);
  if (isObject(value)) {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, canonical(value[key])]),
    );
  }
  return value;
}

/** JSON of the value with object keys sorted at every depth; undefined for undefined. */
export function canonicalJson(value: unknown): string | undefined {
  return JSON.stringify(canonical(value));
}

/** A copy of the card whose `supportedInterfaces` object items carry no `url`. */
export function withoutInterfaceUrls(
  card: Record<string, unknown>,
): Record<string, unknown> {
  const interfaces = card.supportedInterfaces;
  if (!Array.isArray(interfaces)) return { ...card };
  return {
    ...card,
    supportedInterfaces: interfaces.map((item: unknown) => {
      if (!isObject(item)) return item;
      const withoutUrl = { ...item };
      delete withoutUrl.url;
      return withoutUrl;
    }),
  };
}

function canonicallyEqual(a: unknown, b: unknown): boolean {
  return canonicalJson(a) === canonicalJson(b);
}

function indexSkillsByName(skills: unknown): Map<string, JsonObject> {
  return new Map(
    objectsIn(skills).map((skill) => [asString(skill.name), skill]),
  );
}

function differingFields(a: JsonObject, b: JsonObject): string[] {
  const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
  keys.delete("skills");
  return [...keys].filter((key) => !canonicallyEqual(a[key], b[key])).sort();
}

/**
 * Compares two raw A2A cards on the same terms as the backend fingerprint:
 * every list in the result is empty exactly when the cards, without interface URLs,
 * are canonically equal.
 */
export function diffA2ACards(a: unknown, b: unknown): A2ACardDiff {
  const cardA = withoutInterfaceUrls(asObject(a));
  const cardB = withoutInterfaceUrls(asObject(b));
  const skillsA = indexSkillsByName(cardA.skills);
  const skillsB = indexSkillsByName(cardB.skills);

  const added = [...skillsB.keys()].filter((name) => !skillsA.has(name));
  const removed = [...skillsA.keys()].filter((name) => !skillsB.has(name));
  const changed = [...skillsA.keys()].filter(
    (name) =>
      skillsB.has(name) &&
      !canonicallyEqual(skillsA.get(name), skillsB.get(name)),
  );

  const fields = differingFields(cardA, cardB);
  const noSkillDifferenceFound =
    added.length + removed.length + changed.length === 0;
  if (noSkillDifferenceFound && !canonicallyEqual(cardA.skills, cardB.skills)) {
    fields.push("skills");
    fields.sort();
  }
  return { fields, skills: { added, removed, changed } };
}

export function a2aCardsIdentical(diff: A2ACardDiff): boolean {
  return (
    diff.fields.length === 0 &&
    diff.skills.added.length === 0 &&
    diff.skills.removed.length === 0 &&
    diff.skills.changed.length === 0
  );
}
