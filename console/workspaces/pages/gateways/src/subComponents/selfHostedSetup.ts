/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { GatewayTokenInfo } from "@agent-management-platform/types";

const RELEASE_BASE_URL = "https://github.com/wso2/api-platform/releases/download/ai-gateway";
const HELM_CHART_REF = "oci://ghcr.io/wso2/api-platform/helm-charts/gateway";
const ENV_FILE = "api-platform.env";
const ENCRYPTION_KEY_FILE = "default-aesgcm256-v1.bin";
const ENCRYPTION_KEYS_SECRET = "gateway-encryption-keys";

/** Config carries the gateway version with or without the leading "v". */
const bareVersion = (version: string): string => version.replace(/^v/, "");

export interface DockerCommands {
  download: string;
  setup: string;
  configure: string;
  start: string;
}

export function dockerCommands(version: string, cpHost: string, token: string): DockerCommands {
  const bare = bareVersion(version);
  const distribution = `wso2apip-ai-gateway-${bare}`;
  return {
    download: `curl -sLO ${RELEASE_BASE_URL}/v${bare}/${distribution}.zip && unzip ${distribution}.zip`,
    setup: `cd ${distribution} && ./scripts/setup.sh`,
    configure: `cat >> ${ENV_FILE} << 'ENVFILE'
APIP_GW_CONTROLLER_CONTROLPLANE_HOST=${cpHost}
APIP_GW_CONTROLLER_CONTROLPLANE_TOKEN=${token}
ENVFILE`,
    start: "docker compose up",
  };
}

export interface HelmCommands {
  createEncryptionKeySecret: string;
  install: string;
}

export function helmCommands(version: string, cpHost: string, token: string): HelmCommands {
  return {
    createEncryptionKeySecret: `openssl rand 32 > ${ENCRYPTION_KEY_FILE}
kubectl create secret generic ${ENCRYPTION_KEYS_SECRET} --from-file=${ENCRYPTION_KEY_FILE}=${ENCRYPTION_KEY_FILE}`,
    install: `helm install gateway ${HELM_CHART_REF} --version ${bareVersion(version)} \\
  --set gateway.controller.controlPlane.host=${cpHost} \\
  --set gateway.controller.controlPlane.token.value=${token} \\
  --set gateway.controller.encryptionKeys.enabled=true \\
  --set gateway.controller.encryptionKeys.secretName=${ENCRYPTION_KEYS_SECRET}`,
  };
}

/** The gateway dials `host[:port][/path]`, not a URL. */
export function cpHostFromUrl(url: string): string {
  return url.trim().replace(/^https?:\/\//, "");
}

export const activeTokens = (tokens: GatewayTokenInfo[]): GatewayTokenInfo[] =>
  tokens.filter((token) => token.status === "active");

/**
 * Revokes every active token, then mints one. A gateway holds at most two
 * active tokens, so minting first would fail with a conflict.
 */
export async function reconfigureToken(
  listActiveTokenIds: () => Promise<string[]>,
  revoke: (tokenId: string) => Promise<unknown>,
  create: () => Promise<string>,
): Promise<string> {
  const tokenIds = await listActiveTokenIds();
  await Promise.all(tokenIds.map((tokenId) => revoke(tokenId)));
  return create();
}

/**
 * An http(s) URL with a host name, no user information and nothing after the authority
 * but an optional "/": what the backend accepts as a gateway endpoint.
 */
export const isGatewayBaseUrl = (value: string): boolean => {
  const trimmed = value.trim();
  // URL parsing drops an empty "?" or "#", so look for them in the input itself.
  if (trimmed.includes("?") || trimmed.includes("#")) return false;
  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    return false;
  }
  return (
    (url.protocol === "http:" || url.protocol === "https:") &&
    url.hostname !== "" &&
    url.username === "" &&
    url.password === "" &&
    url.pathname === "/"
  );
};

