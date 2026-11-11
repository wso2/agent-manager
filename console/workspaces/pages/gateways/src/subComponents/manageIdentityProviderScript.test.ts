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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { afterEach, describe, expect, it, vi } from "vitest";
import { buildScript } from "./manageIdentityProviderScript";

// The package root also exports the UI components, which do not load under
// vitest; the builder only needs the script helpers.
vi.mock("@agent-management-platform/shared-component", () =>
  import("../../../../libs/shared-component/src/utils/gatewayScripts"),
);

type RuntimeConfig = Record<string, string | undefined>;
const config = (window as unknown as { __RUNTIME_CONFIG__: RuntimeConfig })
  .__RUNTIME_CONFIG__;
const KEYS = ["ampVersion", "ampRegistry", "chartVersion", "apiBaseUrl"];

afterEach(() => KEYS.forEach((key) => delete config[key]));

const inputs = {
  mode: "upsert" as const,
  orgId: "default",
  envName: "dev",
  gatewayId: "gw-1",
  token: "tok",
  name: "corp-idp",
  issuer: "https://idp.example.com",
  jwksUri: "https://idp.example.com/jwks",
  skipTlsVerify: false,
};

describe("buildScript", () => {
  it("pipes the script and leaves the chart to its public default", () => {
    Object.assign(config, { ampVersion: "v2.0.0", apiBaseUrl: "https://api.example.com" });
    const script = buildScript(inputs);
    expect(script.split("\n")[0]).toBe(
      "curl -fsSL 'https://raw.githubusercontent.com/wso2/agent-manager/amp/v2.0.0/deployments/scripts/manage-identity-provider.sh' \\",
    );
    expect(script).toContain("    CHART_VERSION='2.0.0' \\");
    expect(script).not.toContain("CHART_REF");
    expect(script).not.toContain("helm registry login");
  });

  it("signs helm in and upgrades the chart from a private registry", () => {
    Object.assign(config, {
      ampVersion: "v2.0.0",
      chartVersion: "2.0.0-u1",
      ampRegistry: "registry.example.com/proj/sts",
    });
    const lines = buildScript(inputs).split("\n");
    expect(lines[0]).toBe("helm registry login 'registry.example.com' \\");
    expect(lines[1]).toMatch(/^ {2}&& curl -fsSL '.*\/manage-identity-provider\.sh' \\$/);
    expect(lines).toContain("    CHART_VERSION='2.0.0-u1' \\");
    expect(lines).toContain(
      "    CHART_REF='oci://registry.example.com/proj/sts/helm-charts/wso2-amp-api-platform-gateway-extension' \\",
    );
  });
});
