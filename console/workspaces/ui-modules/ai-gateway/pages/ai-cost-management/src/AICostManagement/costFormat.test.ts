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

import type { CostByGroup } from "@agent-management-platform/types";
import {
  zeroCostNote,
  changePercent,
  formatChange,
  dimensionsFor,
  formatUsd,
  formatUsdCents,
  groupKey,
  groupLabel,
  keptMonths,
  monthLabel,
  monthlyTotals,
  unpricedTotals,
} from "./costFormat";

const MONTHS = ["2026-09", "2026-10", "2026-11"];

const entry = (
  over: Partial<CostByGroup>,
  costs = ["0", "0", "0"],
): CostByGroup => ({
  id: null,
  cost: "0",
  unpricedRequests: { missingPrice: 0, usageUnavailable: 0 },
  monthly: MONTHS.map((month, i) => ({ month, cost: costs[i] })),
  ...over,
});

describe("costFormat", () => {
  it("offers only the dimensions below a project on a project's page", () => {
    expect(dimensionsFor(true).map((d) => d.key)).toEqual([
      "agent",
      "apiKey",
      "user",
      "llmProvider",
      "model",
    ]);
    expect(dimensionsFor(false)[0].key).toBe("organization");
  });

  it("keeps cents for small amounts", () => {
    expect(formatUsd(4.6)).toBe("$4.60");
    expect(formatUsd(12840.4)).toBe("$12,840");
  });

  it("labels months in UTC", () => {
    expect(monthLabel("2026-11")).toBe("Nov");
    expect(monthLabel("2026-11", true)).toBe("November 2026");
  });

  it("keys an agent by its project, and names it with the project only on the org page", () => {
    const key = groupKey(
      entry({ id: "hotel-booking", project: "api-platform" }),
      "agent",
    );
    expect(key).toBe("api-platform/hotel-booking");
    expect(groupLabel(key, "agent", false)).toBe(
      "api-platform / hotel-booking",
    );
    expect(groupLabel(key, "agent", true)).toBe("hotel-booking");
  });

  it("shows display names where known, and the handle otherwise", () => {
    const names = {
      llmProvider: { "openai-provider": "OpenAI Provider" },
      project: { "test-project": "Test Project" },
      agent: { "test-project/test": "Test" },
    };
    expect(groupLabel("openai-provider", "llmProvider", false, names)).toBe(
      "OpenAI Provider",
    );
    expect(groupLabel("anthropic", "llmProvider", false, names)).toBe(
      "anthropic",
    );
    expect(groupLabel("test-project", "project", false, names)).toBe(
      "Test Project",
    );
    expect(groupLabel("test-project/test", "agent", false, names)).toBe(
      "Test Project / Test",
    );
    expect(groupLabel("test-project/test", "agent", true, names)).toBe("Test");
  });

  it("calls an entry without a value Unattributed", () => {
    expect(
      groupLabel(
        groupKey(entry({ project: "research" }), "agent"),
        "agent",
        false,
      ),
    ).toBe("Unattributed");
    expect(groupLabel(groupKey(entry({}), "user"), "user", false)).toBe(
      "Unattributed",
    );
    expect(groupLabel("gpt-4o", "model", false)).toBe("gpt-4o");
  });

  it("sums each month, the unpriced requests, and the change from the month before", () => {
    const entries = [
      entry({ unpricedRequests: { missingPrice: 3, usageUnavailable: 0 } }, [
        "10",
        "20.5",
        "1",
      ]),
      entry({ unpricedRequests: { missingPrice: 1, usageUnavailable: 4 } }, [
        "5",
        "4.5",
        "bad",
      ]),
    ];
    expect(monthlyTotals(entries, MONTHS)).toEqual([15, 25, 1]);
    expect(unpricedTotals(entries)).toEqual({
      missingPrice: 4,
      usageUnavailable: 4,
    });
    expect(changePercent(15, 25)).toBe(67);
    expect(changePercent(0, 25)).toBeUndefined();
  });

  it("shows an amount under one cent as less than a cent, and zero as zero", () => {
    for (const v of [0.001, 0.004, 0.009, 0.0099]) {
      expect(formatUsdCents(v)).toBe("<$0.01");
      expect(formatUsd(v)).toBe("<$0.01");
    }
    expect(formatUsdCents(0)).toBe("$0.00");
    expect(formatUsd(0)).toBe("$0.00");
    expect(formatUsdCents(0.01)).toBe("$0.01");
    expect(formatUsd(116.03)).toBe("$116");
    expect(formatUsd(99.5)).toBe("$99.50");
    expect(formatUsd(1234.5)).toBe("$1,235");
  });

  it("writes a change as a signed percentage against a month", () => {
    expect(formatChange(50, "Sep")).toBe("+50% vs Sep");
    expect(formatChange(-12, "Oct")).toBe("-12% vs Oct");
    expect(formatChange(0, "Oct")).toBe("0% vs Oct");
  });

  it("takes the months from the report, else the 3 UTC months up to now", () => {
    expect(keptMonths([entry({})])).toEqual(MONTHS);
    expect(keptMonths([], new Date(Date.UTC(2027, 0, 15)))).toEqual([
      "2026-11",
      "2026-12",
      "2027-01",
    ]);
  });
});

describe("zeroCostNote", () => {
  it("names the groups left out of a chart", () => {
    expect(zeroCostNote(["gpt-4o-nousage", "my-private-model"])).toBe(
      "Not shown (charged $0): gpt-4o-nousage, my-private-model.",
    );
  });
});
