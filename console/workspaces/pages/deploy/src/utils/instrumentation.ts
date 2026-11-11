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

import type { AgentBuildOptions } from "@agent-management-platform/types";

type InstrumentationVersionEntry =
  AgentBuildOptions["instrumentation"]["versions"][number];

/** The agent fields these helpers read. Structural so callers can pass an
 *  AgentResponse without importing its full type. */
type AgentWithBuild = { build?: { type?: string } | null } | null | undefined;

/**
 * Buildpack language of an agent ("python", "ballerina", ...), or undefined for
 * Docker agents and agents whose build is not yet known.
 *
 * Kind-based agents have no build workflow, so the backend reconstructs `build`
 * from the component's language labels — this reads the same field either way.
 */
export function buildpackLanguage(agent: AgentWithBuild): string | undefined {
  const build = agent?.build;
  if (!build || build.type !== "buildpack") return undefined;
  return (build as { buildpack?: { language?: string } }).buildpack?.language;
}

/**
 * Languages the platform can auto-instrument. Both are gated by the same
 * `enableAutoInstrumentation` flag server-side (see buildTraitEnvConfigs), so both
 * should be offered the toggle. Docker agents have no instrumentation trait at all.
 */
export function isInstrumentableLanguage(language?: string): boolean {
  return language === "python" || language === "ballerina";
}

/**
 * Collapse a Python runtime version to its bare `major.minor` form
 * ("3.11.4" -> "3.11"), matching the shape the instrumentation catalog uses.
 * Mirrors the backend's `normalizePythonMinor` so client-side compatibility
 * filtering agrees with server-side validation. Returns undefined when the
 * value has fewer than two dot-separated components.
 */
export function normalizePythonMinor(raw?: string): string | undefined {
  const trimmed = raw?.trim();
  if (!trimmed) return undefined;
  const parts = trimmed.split(".");
  if (parts.length < 2) return undefined;
  const [major, minor] = parts;
  if (!major || !minor) return undefined;
  return `${major}.${minor}`;
}

/**
 * Instrumentation catalog entries compatible with the given (normalized) Python
 * version. When the Python version is unknown (undefined/unnormalizable), returns
 * an empty list rather than every entry — we must not offer or seed a version we
 * cannot verify covers the agent's runtime (the version is baked into an
 * init-container image and a mismatch fails the deploy).
 */
export function compatibleInstrumentationVersions(
  buildOptions: AgentBuildOptions | undefined,
  pythonMinor: string | undefined,
): InstrumentationVersionEntry[] {
  if (!buildOptions || !pythonMinor) return [];
  return buildOptions.instrumentation.versions.filter((v) =>
    v.pythonVersions.includes(pythonMinor),
  );
}

/**
 * Pick the version to seed the selector with: the agent's pinned version if it
 * is still compatible, otherwise the platform default if compatible, otherwise
 * the first compatible entry, otherwise "" (no compatible version — the manual
 * instrumentation fallback). Never returns a version outside `compatible`, so
 * the selector and the redeploy payload never carry a python-incompatible pin.
 */
export function pickInstrumentationVersion(
  compatible: InstrumentationVersionEntry[],
  pinned: string | undefined,
  defaultVersion: string | undefined,
): string {
  const has = (v: string | undefined): v is string =>
    !!v && compatible.some((c) => c.version === v);
  if (has(pinned)) return pinned;
  if (has(defaultVersion)) return defaultVersion;
  return compatible[0]?.version ?? "";
}
