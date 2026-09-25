import {
  useMutation,
  useQuery,
  type QueryKey,
  type UseMutationOptions,
  type UseMutationResult,
  type UseQueryOptions,
  type UseQueryResult,
} from "@tanstack/react-query";
import { useSnackBar } from "@agent-management-platform/views";
import { useEffect, useRef } from "react";
import { useAuthHooks } from "@agent-management-platform/auth";
import { isRequestTooLargeError } from "../utils";

type MutationAction =
  | "assign"
  | "build"
  | "create"
  | "delete"
  | "deploy"
  | "cancel"
  | "generate"
  | "promote"
  | "publish"
  | "remove"
  | "restore"
  | "rerun"
  | "revoke"
  | "rotate"
  | "start"
  | "stop"
  | "undeploy"
  | "unpublish"
  | "update";

type MutationActionConfig = {
  verb: MutationAction;
  target: string;
};

type MessageResolver<TValue, TVariables> =
  | string
  | ((value: TValue, variables: TVariables) => string)
  | undefined;

type ApiMutationOptions<TData, TError, TVariables, TContext> =
  UseMutationOptions<TData, TError, TVariables, TContext> & {
    action?: MutationActionConfig;
    successMessage?: MessageResolver<TData, TVariables>;
    errorMessage?: MessageResolver<TError, TVariables>;
    showSuccess?: boolean;
    showError?: boolean;
  };

const SUCCESS_VERB_MAP: Record<MutationAction, string> = {
  assign: "assigned",
  build: "built",
  cancel: "cancelled",
  create: "created",
  delete: "deleted",
  deploy: "deployed",
  generate: "generated",
  promote: "promoted",
  publish: "published",
  remove: "removed",
  restore: "restored",
  rerun: "triggered",
  revoke: "revoked",
  rotate: "rotated",
  start: "started",
  stop: "stopped",
  undeploy: "undeployed",
  unpublish: "unpublished",
  update: "updated",
};

function toTitleCase(value: string): string {
  return value
    .split(/[\s-_]+/)
    .filter(Boolean)
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(" ");
}

function getQueryTarget(queryKey: QueryKey): string {
  const root = Array.isArray(queryKey) ? queryKey[0] : queryKey;
  return typeof root === "string" ? toTitleCase(root) : "data";
}

function resolveMessage<TValue, TVariables>(
  resolver: MessageResolver<TValue, TVariables>,
  value: TValue,
  variables: TVariables,
): string | undefined {
  if (typeof resolver === "function") {
    return resolver(value, variables);
  }

  return resolver;
}

function getActionSuccessMessage(action: MutationActionConfig): string {
  return `${toTitleCase(action.target)} ${SUCCESS_VERB_MAP[action.verb]} successfully`;
}

// Max `reason` length shown inline in the single-line, non-wrapping snackbar
// (see SnackBar's `noWrap` message Typography) before it's dropped in favor of
// just the base message. Exported so callers pushing their own snackbar directly
// (outside useApiMutation) can bound `reason` the same way.
export const MAX_SNACKBAR_REASON_LENGTH = 20;

/**
 * Extracts a human-readable, server-provided message from a thrown error so it
 * can be surfaced to the user (e.g. a CONFLICT explaining why a delete failed).
 * The http write helpers (`httpPOST`/`httpPUT`/`httpDELETE`) throw a real `Error`
 * whose `message` is the backend's `message` field and whose `.body` is the full
 * parsed error JSON (`{ code, message, reason, additionalData }`) — so `reason`,
 * when the backend sends one, sits under `.body.reason`, not on the error itself.
 * Returns undefined for synthetic transport messages so the caller can fall back
 * to a friendly generic message instead of leaking "HTTP error! status: 500".
 *
 * `maxReasonLength`, when given, drops `reason` from the result once it exceeds
 * that length — for callers (like the single-line, non-wrapping snackbar) whose
 * display has no room for a long reason. Callers with room to wrap (e.g. a
 * drawer's inline Alert) should omit it and show the reason in full.
 */
export function extractServerErrorMessage(
  error: unknown,
  { maxReasonLength }: { maxReasonLength?: number } = {},
): string | undefined {
  if (!error || typeof error !== "object") {
    return undefined;
  }
  const body = (error as { body?: { message?: unknown; reason?: unknown } }).body;
  const candidates: unknown[] = [
    (error as { message?: unknown }).message,
    body?.message,
  ];
  for (const candidate of candidates) {
    if (typeof candidate === "string") {
      const trimmed = candidate.trim();
      if (trimmed && !/^HTTP error! status:/i.test(trimmed)) {
        const reason = typeof body?.reason === "string" ? body.reason.trim() : "";
        const reasonFits =
          reason.length > 0 && (maxReasonLength === undefined || reason.length <= maxReasonLength);
        return reasonFits ? `${trimmed}: ${reason}` : trimmed;
      }
    }
  }
  return undefined;
}

/**
 * Handles auth/session-related failures (may call `logout`) and other cases
 * where a generic error snackbar should not appear. Returns true when the
 * error is considered handled for notification purposes.
 */
function handleAuthAndExpectedErrors(
  error: unknown,
  logout: () => void
): boolean {
  if (
    error &&
    (error as { code?: string })?.code === "SPA-AUTH_CLIENT-VM-NF01"
  ) {
    return true;
  }
  if (
    error &&
    (error as { code?: string })?.code === "SPA-AUTH_CLIENT-VM-IV02"
  ) {
    logout();
    return true;
  }
  const e = error as { status?: number; response?: { status?: number } };
  const status = e.status ?? e.response?.status;
  return status === 401;
}

export type ApiQueryOptions<
  TQueryFnData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = UseQueryOptions<TQueryFnData, TError, TData, TQueryKey> & {
  /**
   * Suppress the error snackbar for every failure of this query. Auth handling
   * (session-expiry logout, 401) still runs. Use for best-effort lookups that
   * only decorate a page — a caller lacking permission for the decoration
   * shouldn't get an error toast on a page whose own content loaded fine.
   */
  silent?: boolean;
};

export function useApiQuery<
  TQueryFnData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>({
  silent = false,
  ...options
}: ApiQueryOptions<TQueryFnData, TError, TData, TQueryKey>): UseQueryResult<TData, TError> {
  const { pushSnackBar } = useSnackBar();
  const { isAuthenticated, logout } = useAuthHooks();
  const query = useQuery(options);
  const lastErrorMessageRef = useRef<string | null>(null);
  const handledErrorRef = useRef<unknown>(undefined);

  useEffect(() => {
    if (!query.isError) {
      lastErrorMessageRef.current = null;
      handledErrorRef.current = undefined;
      return;
    }

    // Nothing to act on without an initialized session. `isAuthenticated` is
    // `isSignedIn && isInitialized`, so it is also false mid-bootstrap, and
    // `logout()` is a hard `window.location.assign` — running either branch
    // here would bounce the user to /login before the app finished starting.
    if (!isAuthenticated) {
      lastErrorMessageRef.current = null;
      return;
    }

    // Act once per distinct error. This effect re-runs on every render for the
    // many callers that build `queryKey` inline (a fresh array each time), and
    // without this an invalidated session would call `logout()` — a redirect —
    // repeatedly while the error sticks around.
    if (handledErrorRef.current === query.error) {
      return;
    }
    handledErrorRef.current = query.error;

    // Auth failures are handled (and can log the user out) even when silent.
    if (handleAuthAndExpectedErrors(query.error, logout)) {
      lastErrorMessageRef.current = null;
      return;
    }

    if (silent) {
      lastErrorMessageRef.current = null;
      return;
    }

    // Determine API call name for error message
    const queryTarget = getQueryTarget(options.queryKey);
    let apiCallName = "data";
    // Map common query targets to user-friendly API call names
    switch (queryTarget.toLowerCase()) {
      case "agent":
        apiCallName = "agent";
        break;
      case "agents":
        apiCallName = "agents";
        break;
      case "project":
        apiCallName = "project";
        break;
      case "projects":
        apiCallName = "projects";
        break;
      case "environment":
        apiCallName = "environment";
        break;
      case "environments":
        apiCallName = "environments";
        break;
      // Add more cases as needed for other API entities
      default:
        apiCallName = queryTarget;
    }

    const fallbackMessage = `Failed to fetch ${apiCallName}`;
    // Always show only the generic message for any HTTP/network error
    const errorMessage = fallbackMessage;

    // Only show if not already shown
    if (lastErrorMessageRef.current === errorMessage) {
      return;
    }

    lastErrorMessageRef.current = errorMessage;
    if ((query.error as { status?: number })?.status === 404) {
      // Intentionally suppress 404 snackbars for optional-resource lookups
      // (for example: feature/existence checks where "not found" is expected UX).
      // Do not rely on this for required-resource queries (detail pages, mandatory
      // config, etc.); those callers should surface explicit UI feedback instead.
      // If a query type needs different behavior, handle 404 in the consuming UI
      // and consider centralizing policy with a query-level option in future.
      return;
    }
    pushSnackBar({ message: errorMessage, type: "error" });
  }, [
    isAuthenticated,
    options.queryKey,
    pushSnackBar,
    query.error,
    query.isError,
    logout,
    silent,
  ]);

  return query;
}

export function useApiMutation<
  TData = unknown,
  TError = unknown,
  TVariables = void,
  TContext = unknown,
>(
  options: ApiMutationOptions<TData, TError, TVariables, TContext>,
): UseMutationResult<TData, TError, TVariables, TContext> {
  const { pushSnackBar } = useSnackBar();
  const { isAuthenticated, logout } = useAuthHooks();
  const {
    action,
    successMessage,
    errorMessage,
    showSuccess = Boolean(action || successMessage),
    showError = true,
    onSuccess,
    onError,
    ...mutationOptions
  } = options;

  return useMutation({
    ...mutationOptions,
    onSuccess: (data, variables, onMutateResult, context) => {
      if (showSuccess && isAuthenticated) {
        pushSnackBar({
          message:
            resolveMessage(successMessage, data, variables) ??
            (action
              ? getActionSuccessMessage(action)
              : "Request completed successfully"),
          type: "success",
        });
      }

      onSuccess?.(data, variables, onMutateResult, context);
    },
    onError: (error, variables, onMutateResult, context) => {
      if (
        showError &&
        isAuthenticated &&
        !handleAuthAndExpectedErrors(error, logout)
      ) {
        // Surface an explicit error message when available, preferring a
        // caller-supplied resolver, then the server-provided message (e.g. a
        // CONFLICT explaining a failed delete), then a friendly generic fallback.
        const subject = action?.target || "data";
        const fallbackMessage = `Failed to submit ${subject}`;
        pushSnackBar({
          // A size refusal is checked first: it names what the user has to
          // change, which a caller's generic failure text would hide.
          message:
            (isRequestTooLargeError(error) ? error.message : undefined) ??
            resolveMessage(errorMessage, error, variables) ??
            extractServerErrorMessage(error, { maxReasonLength: MAX_SNACKBAR_REASON_LENGTH }) ??
            fallbackMessage,
          type: "error",
        });
      }

      onError?.(error, variables, onMutateResult, context);
    },
  });
}
