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

import { cloneElement, useCallback, useRef, type ReactElement } from "react";
import { Tooltip } from "@wso2/oxygen-ui";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";
import type { AccessDecision } from "../../utils/environmentTierAccess";

export interface RestrictedActionProps {
  decision: AccessDecision;
  /**
   * Names the capability this control represents, e.g. "promote-deployment".
   * Reported when a user reaches for a control their scopes do not allow.
   */
  feature?: string;
  /**
   * The control itself. A denial disables it, so the caller's own `disabled`
   * expression carries only its own conditions.
   */
  children: ReactElement<{ disabled?: boolean }>;
}

/**
 * Disables a control the caller's scopes do not reach, and explains why.
 *
 * Owning the `disabled` prop rather than trusting the caller to repeat the
 * decision is what makes the denial unstateable in half: a wrapper without the
 * matching `disabled` would render a live button whose tooltip says it may not
 * be pressed.
 *
 * A disabled control emits no pointer events, so a Tooltip placed directly on it
 * never opens; the reason has to hang off an element beside it. That element is
 * focusable, because a disabled button is skipped by the tab order and a reason
 * only reachable with a pointer is a reason keyboard users never get. Both the
 * span and the clone happen only on a denial, so an allowed control keeps
 * exactly the markup it had.
 *
 * Disabling rather than hiding is deliberate: an absent Promote button is
 * already how the console says "this environment has no downstream target", and
 * a missing permission is a different thing to say.
 */
export function RestrictedAction({
  decision,
  feature,
  children,
}: RestrictedActionProps) {
  const { track } = useTrack();
  // A denied control renders once per list row, so reporting on render would
  // count screens rather than people. What is worth knowing is that someone
  // reached for it: hover or focus is the first observable moment of intent,
  // and it is reported once per mounted control so hovering repeatedly still
  // counts as one attempt.
  const reported = useRef(false);

  const reportAttempt = useCallback(() => {
    if (reported.current) return;
    reported.current = true;
    track(ConsoleAction.PermissionDenied, {
      feature: feature ?? "unspecified",
      // The scope, not decision.reason: the reason is prose written for a
      // tooltip and may name the environment or resource involved.
      required_permission: decision.missingScope ?? "unspecified",
    });
  }, [track, feature, decision.missingScope]);

  if (decision.allowed) return children;
  return (
    <Tooltip title={decision.reason}>
      <span
        tabIndex={0}
        style={{ display: "inline-flex" }}
        onMouseEnter={reportAttempt}
        onFocus={reportAttempt}
      >
        {cloneElement(children, { disabled: true })}
      </span>
    </Tooltip>
  );
}
