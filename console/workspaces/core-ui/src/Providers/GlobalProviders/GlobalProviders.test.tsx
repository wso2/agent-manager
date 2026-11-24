/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import {
  useFeatureAccess,
  useFeatureFlag,
} from "@agent-management-platform/feature-management";
import { PLUGIN_API_VERSION } from "@agent-management-platform/types";
import { useIsUIModuleInstalled } from "@agent-management-platform/views";
import { GlobalProviders } from "./GlobalProviders";


const mocks = vi.hoisted(() => ({
  runtime: {} as {
    data?: { consoleAnalyticsEnabled?: boolean };
    isLoading: boolean;
    isError: boolean;
  },
  analytics: [] as unknown[],
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useRuntimeConfig: () => mocks.runtime,
  setConsoleAnalyticsEnabled: (value: unknown) => {
    mocks.analytics.push(value);
  },
}));

// The real ui-modules are heavy; GlobalProviders only needs a manifest list.
vi.mock("../../plugins/builtinModules", () => ({
  builtinModules: [
    {
      id: "builtin",
      name: "Builtin",
      description: "",
      apiVersion: 1,
      core: true,
      plugins: [],
    },
  ],
}));

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

function Probe() {
  const agentIdentity = useFeatureFlag("enableAgentIdentity");
  const profiles = useFeatureFlag("enableProfileManagement");
  const gatewayWrite = useFeatureAccess("gateway_write");
  const builtin = useIsUIModuleInstalled("builtin");
  const extra = useIsUIModuleInstalled("extra");
  return (
    <div data-testid="probe">
      {JSON.stringify({ agentIdentity, profiles, gatewayWrite, builtin, extra })}
    </div>
  );
}

const probe = () => JSON.parse(screen.getByTestId("probe").textContent ?? "{}");

beforeEach(() => {
  mocks.runtime = { data: { consoleAnalyticsEnabled: true }, isLoading: false, isError: false };
  mocks.analytics.length = 0;
  runtimeConfig.featureFlags = { enableAgentIdentity: true, enableProfileManagement: true };
});

afterEach(() => {
  delete runtimeConfig.featureFlags;
});

describe("GlobalProviders", () => {
  it("holds the app on a loader until runtime config discovery settles", () => {
    mocks.runtime = { isLoading: true, isError: false };
    render(
      <GlobalProviders>
        <Probe />
      </GlobalProviders>,
    );
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
    expect(screen.queryByTestId("probe")).not.toBeInTheDocument();
    expect(mocks.analytics).toEqual([]);
  });

  it("turns console analytics on from the service config before rendering children", () => {
    render(
      <GlobalProviders>
        <Probe />
      </GlobalProviders>,
    );
    expect(mocks.analytics).toEqual([true]);
    expect(screen.getByTestId("probe")).toBeInTheDocument();
  });

  it("leaves analytics off and still renders when discovery fails", () => {
    mocks.runtime = { data: undefined, isLoading: false, isError: true };
    render(
      <GlobalProviders>
        <Probe />
      </GlobalProviders>,
    );
    expect(mocks.analytics).toEqual([undefined]);
    expect(screen.getByTestId("probe")).toBeInTheDocument();
  });

  it("uses the built-in modules by default and lets props override runtime feature flags", () => {
    render(
      <GlobalProviders featureFlags={{ enableAgentIdentity: false }}>
        <Probe />
      </GlobalProviders>,
    );
    expect(probe()).toEqual({
      agentIdentity: false,
      profiles: true,
      gatewayWrite: true,
      builtin: true,
      extra: false,
    });
  });

  it("registers the modules it is given", () => {
    render(
      <GlobalProviders
        modules={[
          { id: "extra", name: "Extra", description: "", apiVersion: PLUGIN_API_VERSION, core: true, plugins: [] },
        ]}
      >
        <Probe />
      </GlobalProviders>,
    );
    expect(probe()).toMatchObject({ builtin: false, extra: true });
  });

  it("waits for the access hook and passes its answer to feature management", () => {
    const { rerender } = render(
      <GlobalProviders useFeatureAccess={() => ({ isLoading: true })}>
        <Probe />
      </GlobalProviders>,
    );
    expect(screen.queryByTestId("probe")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();

    const denyGatewayWrite = () => ({ isLoading: false, featureAccess: { gateway_write: false } });
    rerender(
      <GlobalProviders useFeatureAccess={denyGatewayWrite}>
        <Probe />
      </GlobalProviders>,
    );
    expect(probe().gatewayWrite).toBe(false);
  });
});
