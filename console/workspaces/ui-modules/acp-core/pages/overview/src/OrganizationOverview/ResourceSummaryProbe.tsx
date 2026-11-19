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

import { Component, useLayoutEffect, type ReactNode } from "react";
import type { ResourceSummaryState } from "@agent-management-platform/types";

/**
 * Runs one resource card's `useSummary` hook (contributed by the module that
 * owns the resource) and reports what it returns. Hooks run in their own
 * component so each card's hook has a stable call order whatever cards exist.
 */
export function ResourceSummaryProbe({
  summaryKey,
  useSummary,
  onReport,
}: {
  summaryKey: string;
  useSummary: () => ResourceSummaryState;
  onReport: (key: string, state: ResourceSummaryState) => void;
}) {
  const { count, isLoading, countless, actionLabel, to } = useSummary();
  // A layout effect, so the overview re-renders with the card before the first
  // paint instead of showing an empty card row for a frame.
  useLayoutEffect(() => {
    onReport(summaryKey, { count, isLoading, countless, actionLabel, to });
  }, [summaryKey, count, isLoading, countless, actionLabel, to, onReport]);
  return null;
}

/**
 * A card whose hook throws is left out (and reported through `onFail`, so the
 * overview stops waiting for it); it cannot take the overview down.
 */
export class ResourceSummaryBoundary extends Component<
  { summaryKey: string; onFail: (key: string) => void; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    // eslint-disable-next-line no-console -- surface bad modules to the operator
    console.error(`[overview] Resource summary "${this.props.summaryKey}" threw; leaving it out.`, error);
    this.props.onFail(this.props.summaryKey);
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}
