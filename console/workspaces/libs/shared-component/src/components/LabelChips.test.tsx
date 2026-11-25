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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LabelChips } from "./LabelChips";

// jsdom has no layout, so give the container and chips explicit widths.
function mockLayout(containerWidth: number, chipWidth: number) {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(containerWidth);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: chipWidth,
  } as DOMRect);
}

const visibleChipLabels = (container: HTMLElement) =>
  Array.from(container.firstElementChild!.children)
    .filter((el) => el.getAttribute("aria-hidden") !== "true")
    .map((el) => el.textContent);

describe("LabelChips", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renders nothing without labels", () => {
    const { container } = render(<LabelChips labels={{}} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows every label when they all fit", () => {
    mockLayout(1000, 80);
    const { container } = render(<LabelChips labels={{ env: "prod", tier: "" }} size="medium" />);
    expect(visibleChipLabels(container)).toEqual(["env: prod", "tier"]);
  });

  it("collapses labels that do not fit into a +N chip", () => {
    mockLayout(200, 80);
    const { container } = render(<LabelChips labels={{ a: "1", b: "2", c: "3", d: "4" }} />);
    // 80 + (4 + 80) + reserve(46 + 4) > 200, so only the first fits.
    expect(visibleChipLabels(container)).toEqual(["a: 1", "+3"]);
  });

  it("always keeps one chip visible even when it alone is too wide", () => {
    mockLayout(50, 300);
    const { container } = render(<LabelChips labels={{ long: "value", other: "x" }} />);
    expect(visibleChipLabels(container)).toEqual(["long: value", "+1"]);
  });

  it("keeps the initial layout when the container has no width yet", () => {
    mockLayout(0, 80);
    const { container } = render(<LabelChips labels={{ a: "1", b: "2" }} />);
    expect(visibleChipLabels(container)).toEqual(["a: 1", "b: 2"]);
  });
});
