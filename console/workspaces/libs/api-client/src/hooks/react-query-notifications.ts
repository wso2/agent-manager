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

type MutationAction =
  | "assign"
  | "build"
  | "create"
  | "delete"
  | "deploy"
  | "generate"
  | "remove"
  | "restore"
  | "rerun"
  | "revoke"
  | "rotate"
  | "start"
  | "stop"
  | "undeploy"
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
  create: "created",
  delete: "deleted",
  deploy: "deployed",
  generate: "generated",
  remove: "removed",
  restore: "restored",
  rerun: "triggered",
  revoke: "revoked",
  rotate: "rotated",
  start: "started",
  stop: "stopped",
  undeploy: "undeployed",
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
  if (status === 401) {
    logout();
    return true;
  }
  return status === 400;
}

export function useApiQuery<
  TQueryFnData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(
  options: UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
): UseQueryResult<TData, TError> {
  const { pushSnackBar } = useSnackBar();
  const { isAuthenticated, logout } = useAuthHooks();
  const query = useQuery(options);
  const lastErrorMessageRef = useRef<string | null>(null);

  useEffect(() => {
    if (!query.isError) {
      lastErrorMessageRef.current = null;
      return;
    }

    if (handleAuthAndExpectedErrors(query.error, logout)) {
      lastErrorMessageRef.current = null;
      return;
    }

    if (!isAuthenticated) {
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
    pushSnackBar({ message: errorMessage, type: "error" });
  }, [
    isAuthenticated,
    options.queryKey,
    pushSnackBar,
    query.error,
    query.isError,
    logout,
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
        // Determine subject for error message
        const subject = action?.target || "data";
        // Use a generic message for mutation errors
        const fallbackMessage = `Failed to submit ${subject}`;
        pushSnackBar({
          message: fallbackMessage,
          type: "error",
        });
      }

      onError?.(error, variables, onMutateResult, context);
    },
  });
}
