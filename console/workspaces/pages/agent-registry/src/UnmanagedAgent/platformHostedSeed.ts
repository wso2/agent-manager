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
  INPUT_LIMITS,
  type AgentSourceSeed,
  type ShadowConnectionResponse,
  type ShadowFindingDetail,
} from "@agent-management-platform/types";
import { suggestAgentName } from "@agent-management-platform/shadow-agent-discovery";
import { discoveredDescription } from "./OnboardFinding";

/** The only GitHub API a platform-hosted agent can build from (github.com). */
export const GITHUB_COM_API = "https://api.github.com";

export const NOT_GITHUB_COM =
  "Only repositories on github.com can be deployed as platform-hosted agents.";

/**
 * Why a code agent cannot be deployed as a platform-hosted agent, or undefined
 * when it can. The build clones from github.com only.
 */
export function platformHostedBlocker(
  f: ShadowFindingDetail,
  connection: ShadowConnectionResponse | undefined,
  canCreateAgent: boolean,
  connectionFailed = false,
): string | undefined {
  if (f.provider !== "github") return NOT_GITHUB_COM;
  if (!connection) {
    return connectionFailed
      ? "Could not load the connector's details. Try again later."
      : "Loading the connector details.";
  }
  const api = (connection.endpoints?.api ?? GITHUB_COM_API).replace(/\/+$/, "");
  if (api !== GITHUB_COM_API || !githubOrganization(connection) || !f.scope) {
    return NOT_GITHUB_COM;
  }
  if (!canCreateAgent) return "You don't have permission to create agents.";
  return undefined;
}

function githubOrganization(connection: ShadowConnectionResponse): string {
  const org = connection.settings?.organization;
  return typeof org === "string" ? org.trim() : "";
}

function stringAttribute(f: ShadowFindingDetail, key: string): string {
  const v = f.attributes?.[key];
  return typeof v === "string" ? v.trim() : "";
}

/** The folder of a repository file, as an app path: "/" for the root, else "/folder". */
export function folderAppPath(filePath: string): string {
  const parts = filePath.replace(/\\/g, "/").split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

/** A project path as an app path: a leading "/" and no trailing "/". */
function normalisedAppPath(path: string): string {
  const trimmed = path.replace(/\\/g, "/").replace(/^\/+|\/+$/g, "");
  return trimmed ? `/${trimmed}` : "/";
}

function signalSource(f: ShadowFindingDetail, suffix: string): string {
  return f.signals.find((s) => s.code.endsWith(suffix) && s.source)?.source ?? "";
}

/** The agent's file: the D2 import evidence, else the path part of `<repo>:<path>:<qualified>`. */
export function agentFilePath(f: ShadowFindingDetail): string {
  const fromSignal = signalSource(f, ".d2_agent_import");
  if (fromSignal) return fromSignal;
  const parts = f.externalId.split(":");
  if (parts.length >= 3) return parts.slice(1, -1).join(":");
  return parts[1] ?? "";
}

/** The folder of the agent's project: projectPath, else the manifest's or agent file's folder. */
export function seedAppPath(f: ShadowFindingDetail): string {
  const projectPath = stringAttribute(f, "projectPath");
  if (projectPath) return normalisedAppPath(projectPath);
  const manifest = signalSource(f, ".d1_agent_dependency");
  if (manifest) return folderAppPath(manifest);
  const file = agentFilePath(f);
  return file ? folderAppPath(file) : "/";
}

/** Python and Ballerina have their own build; any other language builds from a Dockerfile. */
export function seedLanguage(f: ShadowFindingDetail): AgentSourceSeed["language"] {
  const language = stringAttribute(f, "language").toLowerCase();
  if (language === "python" || language === "ballerina") return language;
  if (language) return "docker";
  const file = agentFilePath(f).toLowerCase();
  if (file.endsWith(".py")) return "python";
  if (file.endsWith(".bal")) return "ballerina";
  return "docker";
}

/** The values that prefill the platform-hosted agent form for a code agent on github.com. */
export function buildAgentSourceSeed(
  f: ShadowFindingDetail,
  connection: ShadowConnectionResponse,
  orgName: string,
): AgentSourceSeed {
  return {
    displayName: f.displayName.slice(0, INPUT_LIMITS.NAME),
    name: suggestAgentName(f.displayName),
    description: discoveredDescription(f),
    repositoryUrl: `https://github.com/${githubOrganization(connection)}/${f.scope}`,
    branch: stringAttribute(f, "branch") || "main",
    appPath: seedAppPath(f),
    language: seedLanguage(f),
    shadowFinding: { orgName, findingId: f.id },
  };
}
