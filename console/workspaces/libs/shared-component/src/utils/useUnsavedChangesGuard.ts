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

import { useCallback, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { stripBasePath } from "@agent-management-platform/types";
import { useConfirmationDialog } from "../components/ConfirmationDialog/ConfirmationDialogProvider";

type ShowLeaveDialog = (onLeave: () => void) => void;

// Module-level so every mounted guard shares one History API patch and one
// dialog, however many dirty forms (e.g. nested tabs) are on screen.
const activeGuards = new Set<symbol>();
let showLeaveDialog: ShowLeaveDialog | null = null;
let routerNavigate:
  | ((to: string, options: { replace: boolean; state: unknown }) => void)
  | null = null;
let dialogOpen = false;
let bypass = false;
let originalPushState: History["pushState"] | null = null;
let originalReplaceState: History["replaceState"] | null = null;
// React Router stores each entry's position in history.state.idx; tracking the
// current one lets a Back/Forward traversal be measured and undone.
let currentIdx: number | null = null;
let ignoreNextPop = false;
let allowNextPop = false;

function readIdx(state: unknown): number | null {
  const idx = (state as { idx?: unknown } | null)?.idx;
  return typeof idx === "number" ? idx : null;
}

function runUnguarded(action: () => void) {
  bypass = true;
  try {
    action();
  } finally {
    bypass = false;
  }
}

function openLeaveDialog(onLeave: () => void, dialog = showLeaveDialog) {
  if (dialogOpen || !dialog) return;
  dialogOpen = true;
  dialog(() => runUnguarded(onLeave));
}

function handleBeforeUnload(e: BeforeUnloadEvent) {
  // allowNavigation(() => window.location.assign(...)) is a deliberate exit.
  if (bypass) return;
  e.preventDefault();
  e.returnValue = "";
}

// BrowserRouter has no useBlocker, so in-app navigation (links, sidebar,
// navigate(), query-string tab switches) is caught at the History API; browser
// Back/Forward is handled by handlePopState below. A
// blocked push leaves the URL untouched, so the router re-renders in place.
function interceptHistory(replace: boolean): History["pushState"] {
  return (data, unused, url) => {
    const original = (replace ? originalReplaceState : originalPushState)!;
    const passThrough = () => {
      original.call(window.history, data, unused, url);
      currentIdx = readIdx(data) ?? currentIdx;
    };
    if (bypass || url == null) {
      passThrough();
      return;
    }
    const target = new URL(String(url), window.location.href);
    const current = new URL(window.location.href);
    if (target.pathname === current.pathname && target.search === current.search) {
      passThrough();
      return;
    }
    const usr = (data as { usr?: unknown } | null)?.usr;
    // target.pathname is the browser path (base path included); routerNavigate
    // re-applies the basename, so strip it first.
    openLeaveDialog(() =>
      routerNavigate?.(`${stripBasePath(target.pathname)}${target.search}${target.hash}`, {
        replace,
        state: usr,
      }),
    );
  };
}

// Browser Back/Forward fires popstate without going through pushState. Window
// listeners run in registration order (capture doesn't jump the queue there),
// so this is registered at module load — before BrowserRouter mounts and adds
// its own — letting it hide the traversal from the router, step back to where
// the user was, and replay the traversal only once they choose Leave.
function handlePopState(e: PopStateEvent) {
  if (ignoreNextPop) {
    ignoreNextPop = false;
    e.stopImmediatePropagation();
    return;
  }
  const nextIdx = readIdx(e.state);
  if (allowNextPop || activeGuards.size === 0 || currentIdx === null || nextIdx === null) {
    allowNextPop = false;
    // An entry without an idx (e.g. a hash-anchor click) keeps the last known
    // position rather than switching the Back/Forward guard off.
    currentIdx = nextIdx ?? currentIdx;
    return;
  }
  const delta = currentIdx - nextIdx;
  if (delta === 0) return;
  e.stopImmediatePropagation();
  ignoreNextPop = true;
  window.history.go(delta);
  openLeaveDialog(() => {
    allowNextPop = true;
    window.history.go(-delta);
  });
}

if (typeof window !== "undefined") {
  window.addEventListener("popstate", handlePopState);
}

function install() {
  currentIdx = readIdx(window.history.state);
  ignoreNextPop = false;
  allowNextPop = false;
  // Kept unbound (and called with window.history) so uninstall restores the
  // exact original functions rather than a new bound wrapper each cycle.
  originalPushState = window.history.pushState;
  originalReplaceState = window.history.replaceState;
  window.history.pushState = interceptHistory(false);
  window.history.replaceState = interceptHistory(true);
  window.addEventListener("beforeunload", handleBeforeUnload);
}

function uninstall() {
  if (originalPushState) window.history.pushState = originalPushState;
  if (originalReplaceState) window.history.replaceState = originalReplaceState;
  originalPushState = null;
  originalReplaceState = null;
  window.removeEventListener("beforeunload", handleBeforeUnload);
}

function useLeaveDialog(): ShowLeaveDialog {
  const { addConfirmation } = useConfirmationDialog();
  return useCallback(
    (onLeave: () => void) =>
      addConfirmation({
        title: "You have unsaved changes",
        description:
          "Your changes haven't been saved. If you leave now, they will be lost.",
        confirmButtonText: "Leave",
        confirmButtonColor: "error",
        cancelButtonText: "Stay",
        onConfirm: () => {
          dialogOpen = false;
          onLeave();
        },
        onCancel: () => {
          dialogOpen = false;
        },
      }),
    [addConfirmation],
  );
}

/**
 * Asks "You have unsaved changes — Stay / Leave?" before the user navigates
 * away from a form while `isDirty` is true, and shows the browser's native
 * prompt on reload or tab close.
 *
 * Call `allowNavigation(fn)` around a navigation the form triggers itself
 * (e.g. redirecting after a successful save), since `isDirty` is typically
 * still true in that same tick.
 */
export function useUnsavedChangesGuard(isDirty: boolean) {
  const navigate = useNavigate();
  const leaveDialog = useLeaveDialog();
  const idRef = useRef(Symbol("unsaved-changes-guard"));
  // useNavigate's identity changes with the location; reading it through a ref
  // keeps location changes from reinstalling the patch (and resetting the
  // popstate flags) mid-traversal.
  const navigateRef = useRef(navigate);
  navigateRef.current = navigate;

  useEffect(() => {
    if (!isDirty) return undefined;
    const id = idRef.current;
    showLeaveDialog = leaveDialog;
    routerNavigate = (to, options) => navigateRef.current(to, options);
    if (activeGuards.size === 0) install();
    activeGuards.add(id);
    return () => {
      activeGuards.delete(id);
      if (activeGuards.size === 0) uninstall();
    };
  }, [isDirty, leaveDialog]);

  return { allowNavigation: runUnguarded };
}

/**
 * For in-page switches that don't change the URL (e.g. tabs held in local
 * state, closing a drawer): runs `action` straight away when nothing is dirty,
 * otherwise only after the user chooses Leave. Pass `isDirty` to check just
 * the caller's own form; without it, any active guard counts.
 */
export function useConfirmIfUnsaved() {
  const leaveDialog = useLeaveDialog();
  return useCallback(
    (action: () => void, isDirty?: boolean) => {
      if (isDirty === false || (isDirty === undefined && activeGuards.size === 0)) {
        action();
        return;
      }
      openLeaveDialog(action, leaveDialog);
    },
    [leaveDialog],
  );
}
