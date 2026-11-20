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
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { GuardrailDefinition } from "@agent-management-platform/api-client";

export const policy = (
  overrides: Partial<GuardrailDefinition> = {},
): GuardrailDefinition => ({
  name: "pii-masking",
  version: "v1.0.0",
  displayName: "PII Masking",
  description: "Masks PII",
  provider: "hub",
  categories: [],
  isLatest: true,
  source: "hub",
  status: "installed",
  parameters: {
    type: "object",
    properties: { blockedWord: { type: "string" } },
  },
  ...overrides,
});

export const renderWithProviders = (ui: ReactElement) => {
  const result = render(<MemoryRouter>{ui}</MemoryRouter>);
  return {
    ...result,
    // Keep the router around the tree when a test re-renders with new props.
    rerender: (next: ReactElement) =>
      result.rerender(<MemoryRouter>{next}</MemoryRouter>),
  };
};
