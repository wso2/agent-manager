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

import { globalConfig } from "@agent-management-platform/types";

/**
 * The configured Agent Manager release version (e.g. "v0.15.0"). The deployment
 * scripts live in the Agent Manager repo and are tagged with its release, so the
 * script ref is pinned to this version — not the gateway chart version. Empty
 * when not injected (dev), which falls back to `main`.
 */
export function getAmpVersion(): string {
  return globalConfig.ampVersion?.trim() || "";
}

/**
 * Agent Manager release version without the leading "v", for contexts that need
 * bare semver — e.g. the `CHART_VERSION` passed to the gateway-extension OCI chart
 * (OCI tags are plain semver like "0.17.0-rc4", not "v0.17.0-rc4"). Empty when the
 * version is not injected (dev).
 */
export function getAmpVersionHelm(): string {
  const v = getAmpVersion();
  return v.startsWith("v") ? v.slice(1) : v;
}

/**
 * Git ref used to fetch deployment scripts from raw.githubusercontent.com.
 * Release tags are `amp/vX.Y.Z`, so a versioned build pins to that tag. An unset
 * or dev placeholder version (e.g. `0.0.0-dev`) has no matching tag, so fall back
 * to `main`. The leading "v" is normalized so both "v0.15.0" and "0.15.0" work.
 */
export function getScriptRef(): string {
  const version = getAmpVersion();
  if (!version || version.includes("dev")) return "main";
  const bare = version.startsWith("v") ? version.slice(1) : version;
  return `amp/v${bare}`;
}

/**
 * Base URL the generated commands fetch deployment scripts from, without a
 * trailing slash. Defaults to the public raw.githubusercontent.com path for the
 * current release ref; `globalConfig.scriptBaseUrl` overrides it for a
 * deployment where that host is not reachable or serves nothing.
 */
export function getScriptBaseUrl(): string {
  const configured = globalConfig.scriptBaseUrl?.trim();
  if (configured) return configured.replace(/\/+$/, "");
  return `https://raw.githubusercontent.com/wso2/agent-manager/${getScriptRef()}/deployments/scripts`;
}

/** Full URL for a deployment script, pinned to the current release ref. */
export function getRawScriptUrl(scriptName: string): string {
  return `${getScriptBaseUrl()}/${scriptName}`;
}

/**
 * Private registry the platform was installed from, as a host and path prefix
 * (e.g. "registry.example.com/my-project"). Empty for the public registry, where
 * the generated commands keep the scripts' own public defaults.
 */
export function getAmpRegistry(): string {
  return globalConfig.ampRegistry?.trim().replace(/\/+$/, "") || "";
}

/** Helm chart registry of a private install; `<registry>/helm-charts` when not configured. */
export function getAmpChartRegistry(): string {
  const configured = globalConfig.ampChartRegistry?.trim().replace(/\/+$/, "");
  if (configured) return configured;
  const registry = getAmpRegistry();
  return registry ? `${registry}/helm-charts` : "";
}

/**
 * Version of the installed Agent Manager chart, which an environment's gateway
 * extension chart is pinned to. An update moves it past the release version
 * (2.0.0-u1 against 2.0.0), so it is preferred over AMP_VERSION when injected.
 */
export function getInstalledChartVersion(): string {
  return globalConfig.chartVersion?.trim() || getAmpVersionHelm();
}

/**
 * Release bundle a private-registry environment command extracts the scripts
 * from: the GitHub release asset for this version, unless one is configured.
 * Empty for a dev build, which has no release.
 */
export function getReleaseBundleUrl(): string {
  const configured = globalConfig.bundleUrl?.trim();
  if (configured) return configured;
  const ref = getScriptRef();
  if (ref === "main") return "";
  const version = ref.slice("amp/v".length);
  return `https://github.com/wso2/agent-manager/releases/download/${ref}/wso2-agent-manager-${version}.tar.gz`;
}

const GATEWAY_EXTENSION_CHART = "wso2-amp-api-platform-gateway-extension";

/**
 * Variables that point a deployment script at a private registry: images, the
 * gateway, and the gateway extension chart. Empty for a public install.
 */
export function getPrivateRegistryEnv(): Record<string, string> {
  const registry = getAmpRegistry();
  if (!registry) return {};
  const chartRegistry = getAmpChartRegistry();
  return {
    AMP_IMAGE_REGISTRY: registry,
    GATEWAY_REGISTRY: registry,
    HELM_CHART_REGISTRY: chartRegistry,
    CHART_REF: `oci://${chartRegistry}/${GATEWAY_EXTENSION_CHART}`,
  };
}

/** Host of the private chart registry, for `helm registry login`. Empty when public. */
export function getAmpChartRegistryHost(): string {
  return getAmpChartRegistry().split("/")[0] ?? "";
}

/** Host of the private image registry. Empty when public. */
export function getAmpRegistryHost(): string {
  return getAmpRegistry().split("/")[0] ?? "";
}

/** Quote a value as one POSIX shell word without evaluating metacharacters. */
export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

/**
 * Wraps a deployment script's environment lines (each "VAR=value \\") into the
 * command that runs it. A public install pipes the release-pinned script into
 * bash. A private install cannot: the script has to create image pull secrets,
 * which needs lib-registry-auth.sh beside it and a terminal to ask for the
 * registry credentials, so it runs from the extracted release bundle instead,
 * with the scripts it chains into read from the bundle too.
 */
export function buildScriptCommand(
  scriptName: string,
  envLines: string[],
  { fromBundle }: { fromBundle: boolean },
): string {
  if (!fromBundle) {
    return [`curl -fsSL ${getRawScriptUrl(scriptName)} \\`, ...withPrefix("|", envLines), "    bash"].join("\n");
  }
  // A dev build has no release bundle; leave a placeholder rather than pipe a
  // script that cannot create the pull secrets.
  const bundleUrl = getReleaseBundleUrl() || "<release-bundle-url>";
  // The script signs helm in to the image registry's host only, so a chart
  // registry on another host needs its own sign-in first.
  const chartHost = getAmpChartRegistryHost();
  const login = chartHost && chartHost !== getAmpRegistryHost();
  return [
    ...(login ? [`helm registry login ${shellQuote(chartHost)} \\`] : []),
    `${login ? "  && " : ""}AMP_BUNDLE_DIR="$(mktemp -d)" \\`,
    `  && curl -fsSL ${shellQuote(bundleUrl)} | tar -xz -C "$AMP_BUNDLE_DIR" \\`,
    ...withPrefix("&&", [...envLines, `    SCRIPT_BASE_URL="file://$AMP_BUNDLE_DIR/deployments/scripts" \\`]),
    `    bash "$AMP_BUNDLE_DIR/deployments/scripts/${scriptName}"`,
  ].join("\n");
}

function withPrefix(prefix: string, envLines: string[]): string[] {
  return envLines.map((line, i) => (i === 0 ? `  ${prefix} ${line.trimStart()}` : line));
}

const DEFAULT_AGENT_MANAGER_URL = "http://api.amp.localhost:8080";

/** Slash stripped because the scripts append /api/v1. */
export function getAgentManagerUrl(): string {
  const url = globalConfig.apiBaseUrl?.trim().replace(/\/+$/, "");
  return url || DEFAULT_AGENT_MANAGER_URL;
}
