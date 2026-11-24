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

import type { ReactElement } from "react";
import { render, type RenderResult } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import type { LLMProviderResponse } from "@agent-management-platform/types";

/** Prints where the router ended up, so tests can assert navigation. */
export const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname}${location.search}`}</div>;
};

export interface RenderOptions {
  /** Initial URL. */
  path?: string;
  /** Route pattern the element is mounted at. */
  route?: string;
  /** Access overrides, e.g. `{ llm_provider_write: false }`. */
  access?: Record<string, boolean>;
}

export const renderWithProviders = (
  element: ReactElement,
  {
    path = "/org/acme/llm-providers/view/p-1",
    route = "/org/:orgId/llm-providers/view/:providerId",
    access = {},
  }: RenderOptions = {},
): RenderResult & { rerenderWith: (el: ReactElement) => void } => {
  const tree = (el: ReactElement) => (
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <ThemeProvider theme={createTheme()}>
        <FeatureManagementProvider featureAccess={access}>
          <ConfirmationDialogProvider>
            <Routes>
              <Route path={route} element={el} />
              <Route path="*" element={<Destination />} />
            </Routes>
          </ConfirmationDialogProvider>
        </FeatureManagementProvider>
      </ThemeProvider>
    </MemoryRouter>
  );
  const view = render(tree(element));
  return { ...view, rerenderWith: (el: ReactElement) => view.rerender(tree(el)) };
};

export const makeProvider = (
  overrides: Partial<LLMProviderResponse> = {},
): LLMProviderResponse => ({
  uuid: "p-1",
  id: "openai-prod",
  name: "OpenAI Prod",
  version: "v1.0",
  context: "/openai",
  template: "openai",
  upstream: {
    main: {
      url: "https://api.openai.com/v1",
      auth: { type: "api-key", header: "Authorization", value: "Bearer secret" },
    },
  },
  ...overrides,
});

/** A minimal OpenAPI document with the given `METHOD /path` operations. */
export const makeSpec = (ops: string[]): string => {
  const paths: Record<string, Record<string, unknown>> = {};
  for (const op of ops) {
    const [method, path] = op.split(" ");
    paths[path] = {
      ...(paths[path] ?? {}),
      [method.toLowerCase()]: { summary: `${method} ${path} summary`, responses: {} },
    };
  }
  return JSON.stringify({ openapi: "3.0.0", info: { title: "t", version: "1" }, paths });
};
