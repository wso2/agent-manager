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
import type { UIModuleManifest } from "@agent-management-platform/types";
import CoreUI from "./CoreUI";

const received = vi.hoisted(() => ({ props: [] as Record<string, unknown>[] }));

const builtin = vi.hoisted(
  () => [{ id: "builtin", name: "B", description: "", apiVersion: 1, plugins: [] }],
);

vi.mock("./plugins", () => ({ builtinModules: builtin }));

vi.mock("./Providers", () => ({
  GlobalProviders: ({ children, ...props }: { children: ReactNode }) => {
    received.props.push(props);
    return <div data-testid="providers">{children}</div>;
  },
}));

vi.mock("./Route", () => ({ RootRouter: () => <div>router</div> }));

beforeEach(() => {
  received.props.length = 0;
});

describe("CoreUI", () => {
  it("renders the router inside the global providers with the built-in modules", () => {
    render(<CoreUI />);
    expect(screen.getByTestId("providers")).toHaveTextContent("router");
    expect(received.props[0].modules).toBe(builtin);
  });

  it("adds extra modules after the built-in ones and passes the rest through", () => {
    const extra: UIModuleManifest[] = [
      { id: "extra", name: "E", description: "", apiVersion: 1, plugins: [] },
    ];
    const useFeatureAccess = () => ({ isLoading: false });
    const featureFlags = { enableAgentIdentity: true };
    render(
      <CoreUI modules={extra} featureFlags={featureFlags} useFeatureAccess={useFeatureAccess} />,
    );
    const props = received.props[0];
    expect((props.modules as UIModuleManifest[]).map((m) => m.id)).toEqual(["builtin", "extra"]);
    expect(props.featureFlags).toBe(featureFlags);
    expect(props.useFeatureAccess).toBe(useFeatureAccess);
  });

  it("keeps the built-in list when given an empty module list", () => {
    render(<CoreUI modules={[]} />);
    expect(received.props[0].modules).toBe(builtin);
  });
});
