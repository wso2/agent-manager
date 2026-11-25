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

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { formatPolicyVersion } from "./formatPolicyVersion";
import { getMethodChipColor, getResourceKey } from "./openapiResources";
import {
  resolveProviderStatusColor,
  resolveProviderStatusIcon,
  resolveProviderStatusLabel,
} from "./providerStatus";

describe("formatPolicyVersion", () => {
  it("renders exactly one v prefix", () => {
    expect(formatPolicyVersion("1.0.1")).toBe("v1.0.1");
    expect(formatPolicyVersion("v1.0.1")).toBe("v1.0.1");
    expect(formatPolicyVersion("V2")).toBe("v2");
  });
});

describe("openapiResources", () => {
  it("keys a resource by method and path", () => {
    expect(getResourceKey({ method: "GET", path: "/models" })).toBe("GET::/models");
    expect(getResourceKey({ method: "POST", path: "/chat" }, "-")).toBe("POST-/chat");
  });

  it("colours methods", () => {
    expect(getMethodChipColor("get")).toBe("info");
    expect(getMethodChipColor("POST")).toBe("success");
    expect(getMethodChipColor("DELETE")).toBe("error");
    expect(getMethodChipColor("PATCH")).toBe("default");
  });
});

describe("providerStatus", () => {
  it.each([
    ["active", "success", "Active"],
    ["DEPLOYED", "success", "Active"],
    ["pending", "warning", "Pending"],
    ["failed", "error", "Failed"],
    ["archived", "default", "Archived"],
    [undefined, "default", "Unknown"],
  ])("resolves %s", (status, color, label) => {
    expect(resolveProviderStatusColor(status)).toBe(color);
    expect(resolveProviderStatusLabel(status)).toBe(label);
  });

  it("shows a spinner only for pending", () => {
    const { rerender } = render(resolveProviderStatusIcon("pending"));
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
    for (const status of ["active", "failed", undefined]) {
      rerender(resolveProviderStatusIcon(status));
      expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    }
  });
});
