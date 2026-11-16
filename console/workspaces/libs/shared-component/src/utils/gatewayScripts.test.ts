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

import { afterEach, describe, expect, it } from "vitest";
import {
  buildScriptCommand,
  getAmpChartRegistry,
  getAmpChartRegistryHost,
  getInstalledChartVersion,
  getPrivateRegistryEnv,
  getReleaseBundleUrl,
} from "./gatewayScripts";

// The helpers read window.__RUNTIME_CONFIG__ (config.js) through the object
// setupTests.tsx creates, so each case sets its keys on that same object.
type RuntimeConfig = Record<string, string | undefined>;
const config = (window as unknown as { __RUNTIME_CONFIG__: RuntimeConfig })
  .__RUNTIME_CONFIG__;
const KEYS = ["ampVersion", "ampRegistry", "ampChartRegistry", "bundleUrl", "chartVersion", "scriptBaseUrl"];
const set = (values: RuntimeConfig) => Object.assign(config, values);

afterEach(() => KEYS.forEach((key) => delete config[key]));

describe("private registry settings", () => {
  it("are empty for a public install", () => {
    set({ ampVersion: "v2.0.0" });
    expect(getPrivateRegistryEnv()).toEqual({});
    expect(getAmpChartRegistry()).toBe("");
    expect(getAmpChartRegistryHost()).toBe("");
  });

  it("point images, the gateway and the extension chart at the registry", () => {
    set({ ampRegistry: "registry.example.com/proj/sts/" });
    expect(getPrivateRegistryEnv()).toEqual({
      AMP_IMAGE_REGISTRY: "registry.example.com/proj/sts",
      GATEWAY_REGISTRY: "registry.example.com/proj/sts",
      HELM_CHART_REGISTRY: "registry.example.com/proj/sts/helm-charts",
      CHART_REF: "oci://registry.example.com/proj/sts/helm-charts/wso2-amp-api-platform-gateway-extension",
    });
    expect(getAmpChartRegistryHost()).toBe("registry.example.com");
  });

  it("take a configured chart registry over the derived one", () => {
    set({ ampRegistry: "registry.example.com/proj", ampChartRegistry: "charts.example.com/proj" });
    expect(getPrivateRegistryEnv().CHART_REF).toBe(
      "oci://charts.example.com/proj/wso2-amp-api-platform-gateway-extension",
    );
    expect(getAmpChartRegistryHost()).toBe("charts.example.com");
  });
});

describe("getInstalledChartVersion", () => {
  it("prefers the installed chart version, which an update moves", () => {
    set({ ampVersion: "v2.0.0", chartVersion: "2.0.0-u1" });
    expect(getInstalledChartVersion()).toBe("2.0.0-u1");
  });

  it("falls back to the release version", () => {
    set({ ampVersion: "v2.0.0" });
    expect(getInstalledChartVersion()).toBe("2.0.0");
  });
});

describe("getReleaseBundleUrl", () => {
  it("is the GitHub release asset for the version", () => {
    set({ ampVersion: "v2.0.0" });
    expect(getReleaseBundleUrl()).toBe(
      "https://github.com/wso2/agent-manager/releases/download/amp/v2.0.0/wso2-agent-manager-2.0.0.tar.gz",
    );
  });

  it("is empty for a dev build, which has no release", () => {
    set({ ampVersion: "0.0.0-dev" });
    expect(getReleaseBundleUrl()).toBe("");
  });

  it("takes a configured mirror", () => {
    set({ ampVersion: "v2.0.0", bundleUrl: "https://mirror.example.com/b.tar.gz" });
    expect(getReleaseBundleUrl()).toBe("https://mirror.example.com/b.tar.gz");
  });
});

describe("buildScriptCommand", () => {
  const env = ["    ENV_NAME=dev \\", "    TLS_ENABLED=true \\"];

  it("pipes the release-pinned script into bash", () => {
    set({ ampVersion: "v2.0.0" });
    expect(buildScriptCommand("add-environment.sh", env, { fromBundle: false })).toBe(
      [
        "curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/amp/v2.0.0/deployments/scripts/add-environment.sh \\",
        "  | ENV_NAME=dev \\",
        "    TLS_ENABLED=true \\",
        "    bash",
      ].join("\n"),
    );
  });

  it("runs the script and its chained scripts from the extracted release bundle", () => {
    set({ ampVersion: "v2.0.0", ampRegistry: "registry.example.com/proj/sts" });
    expect(buildScriptCommand("add-environment.sh", env, { fromBundle: true })).toBe(
      [
        'AMP_BUNDLE_DIR="$(mktemp -d)" \\',
        "  && curl -fsSL 'https://github.com/wso2/agent-manager/releases/download/amp/v2.0.0/wso2-agent-manager-2.0.0.tar.gz' | tar -xz -C \"$AMP_BUNDLE_DIR\" \\",
        "  && ENV_NAME=dev \\",
        "    TLS_ENABLED=true \\",
        '    SCRIPT_BASE_URL="file://$AMP_BUNDLE_DIR/deployments/scripts" \\',
        '    bash "$AMP_BUNDLE_DIR/deployments/scripts/add-environment.sh"',
      ].join("\n"),
    );
  });

  it("quotes a configured bundle URL as one shell word", () => {
    set({ ampVersion: "v2.0.0", bundleUrl: "https://mirror.example.com/b.tar.gz?sig=a&exp=1" });
    expect(buildScriptCommand("add-environment.sh", env, { fromBundle: true })).toContain(
      "curl -fsSL 'https://mirror.example.com/b.tar.gz?sig=a&exp=1' | tar",
    );
  });

  it("leaves a placeholder rather than pipe when there is no bundle to run from", () => {
    set({ ampVersion: "0.0.0-dev", ampRegistry: "registry.example.com/proj" });
    const command = buildScriptCommand("add-environment.sh", env, { fromBundle: true });
    expect(command).toContain("curl -fsSL '<release-bundle-url>' | tar");
    expect(command).not.toContain("raw.githubusercontent.com");
  });

  it("signs helm in to a chart registry on another host first", () => {
    set({
      ampVersion: "v2.0.0",
      ampRegistry: "registry.example.com/proj",
      ampChartRegistry: "charts.example.com/proj",
    });
    const lines = buildScriptCommand("add-environment.sh", env, { fromBundle: true }).split("\n");
    expect(lines[0]).toBe("helm registry login 'charts.example.com' \\");
    expect(lines[1]).toBe('  && AMP_BUNDLE_DIR="$(mktemp -d)" \\');
  });

  it("leaves the sign-in to the script when charts share the image host", () => {
    set({ ampVersion: "v2.0.0", ampRegistry: "registry.example.com/proj" });
    expect(buildScriptCommand("add-environment.sh", env, { fromBundle: true })).not.toContain(
      "helm registry login",
    );
  });
});
