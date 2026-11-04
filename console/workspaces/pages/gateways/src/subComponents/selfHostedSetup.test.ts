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

import { describe, expect, it, vi } from "vitest";
import {
  activeTokens,
  cpHostFromUrl,
  dockerCommands,
  helmCommands,
  isGatewayBaseUrl,
  reconfigureToken,
} from "./selfHostedSetup";

const HOST = "cp.example.com/agent-manager-internal-api";
const TOKEN = "tok-123";

describe("dockerCommands", () => {
  it.each(["2026.09.24", "v2026.09.24"])(
    "builds the download command from version %s",
    (version) => {
      expect(dockerCommands(version, HOST, TOKEN).download).toBe(
        "curl -sLO https://github.com/wso2/api-platform/releases/download/ai-gateway/v2026.09.24/wso2apip-ai-gateway-2026.09.24.zip && unzip wso2apip-ai-gateway-2026.09.24.zip",
      );
    },
  );

  it("appends the control plane host and token to api-platform.env", () => {
    const { configure } = dockerCommands("2026.09.24", HOST, TOKEN);
    expect(configure).toContain("api-platform.env");
    expect(configure).toContain(`APIP_GW_CONTROLLER_CONTROLPLANE_HOST=${HOST}`);
    expect(configure).toContain(`APIP_GW_CONTROLLER_CONTROLPLANE_TOKEN=${TOKEN}`);
  });

  it("runs setup.sh before the configure step", () => {
    expect(dockerCommands("2026.09.24", HOST, TOKEN).setup).toBe(
      "cd wso2apip-ai-gateway-2026.09.24 && ./scripts/setup.sh",
    );
  });

  it("starts with docker compose up", () => {
    expect(dockerCommands("2026.09.24", HOST, TOKEN).start).toBe("docker compose up");
  });
});

describe("helmCommands", () => {
  it("creates the encryption key secret from a generated key", () => {
    expect(helmCommands("2026.09.24", HOST, TOKEN).createEncryptionKeySecret).toBe(
      "openssl rand 32 > default-aesgcm256-v1.bin\n" +
        "kubectl create secret generic gateway-encryption-keys --from-file=default-aesgcm256-v1.bin=default-aesgcm256-v1.bin",
    );
  });

  it.each(["2026.09.24", "v2026.09.24"])("builds the install for version %s", (version) => {
    const { install } = helmCommands(version, HOST, TOKEN);
    expect(install).toContain(
      "oci://ghcr.io/wso2/api-platform/helm-charts/gateway --version 2026.09.24",
    );
    expect(install).toContain(`--set gateway.controller.controlPlane.host=${HOST}`);
    expect(install).toContain(`--set gateway.controller.controlPlane.token.value=${TOKEN}`);
    expect(install).toContain("--set gateway.controller.encryptionKeys.enabled=true");
    expect(install).toContain(
      "--set gateway.controller.encryptionKeys.secretName=gateway-encryption-keys",
    );
  });

  it("does not set a control plane port", () => {
    expect(helmCommands("2026.09.24", HOST, TOKEN).install).not.toContain("controlPlane.port");
  });
});

describe("cpHostFromUrl", () => {
  it("strips the scheme and keeps host and path", () => {
    expect(
      cpHostFromUrl("https://dev-wso2cloud.gateway.x/agent-manager-service-agent-manager-internal-api"),
    ).toBe("dev-wso2cloud.gateway.x/agent-manager-service-agent-manager-internal-api");
  });

  it("keeps a port", () => {
    expect(cpHostFromUrl("http://localhost:9243")).toBe("localhost:9243");
  });
});

describe("reconfigureToken", () => {
  it("revokes every active token before creating the new one", async () => {
    const calls: string[] = [];
    const list = vi.fn(async () => ["a", "b"]);
    const revoke = vi.fn(async (id: string) => {
      calls.push(`revoke:${id}`);
    });
    const create = vi.fn(async () => {
      calls.push("create");
      return "new-token";
    });

    await expect(reconfigureToken(list, revoke, create)).resolves.toBe("new-token");
    expect(calls).toEqual(["revoke:a", "revoke:b", "create"]);
  });

  it("does not create a token when a revoke fails", async () => {
    const create = vi.fn(async () => "new-token");
    await expect(
      reconfigureToken(
        async () => ["a"],
        async () => {
          throw new Error("boom");
        },
        create,
      ),
    ).rejects.toThrow("boom");
    expect(create).not.toHaveBeenCalled();
  });
});

describe("isGatewayBaseUrl", () => {
  it.each([
    "https://gateway.example.com",
    "https://gateway.example.com/",
    " http://gw.example.com:8443 ",
    "https://10.0.0.5",
  ])("accepts %s", (value) => {
    expect(isGatewayBaseUrl(value)).toBe(true);
  });

  it.each([
    "http://:80",
    "ftp://gw.example.com",
    "gw.example.com",
    "https://",
    "https://user:pw@gw.example.com",
    "https://user@gw.example.com",
    "https://gw.example.com/base",
    "https://gw.example.com//",
    "https://gw.example.com?a=1",
    "https://gw.example.com?",
    "https://gw.example.com/?",
    "https://gw.example.com#top",
    "https://gw.example.com#",
    "",
  ])("rejects %s", (value) => {
    expect(isGatewayBaseUrl(value)).toBe(false);
  });
});

describe("activeTokens", () => {
  it("keeps only active tokens", () => {
    const active = { id: "a", status: "active" as const, createdAt: "" };
    const revoked = { id: "r", status: "revoked" as const, createdAt: "" };
    expect(activeTokens([active, revoked])).toEqual([active]);
  });
});
