/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ConfigSection } from "./ConfigSection";

const heading = (name: RegExp) => screen.getByRole("button", { name });

describe("ConfigSection", () => {
  // Closed, a section shows just its heading and current value.
  it("starts closed with its title and summary showing", () => {
    render(
      <ConfigSection title="File Mounts" summary="2 files">
        <span>file editor</span>
      </ConfigSection>,
    );
    expect(heading(/File Mounts/)).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("2 files")).toBeVisible();
  });

  it("opens on load when asked, and opens and closes on click", () => {
    render(
      <ConfigSection title="Environment Variables" summary="None" defaultExpanded>
        <span>env editor</span>
      </ConfigSection>,
    );
    expect(heading(/Environment Variables/)).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(heading(/Environment Variables/));
    expect(heading(/Environment Variables/)).toHaveAttribute("aria-expanded", "false");
  });

  // An invalid value can't hide in a closed section.
  it("opens itself and says so when its value is invalid", () => {
    render(
      <ConfigSection title="Request Timeout" summary="30 seconds" hasError>
        <span>timeout field</span>
      </ConfigSection>,
    );
    expect(heading(/Request Timeout/)).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Needs attention")).toBeVisible();
  });
});
