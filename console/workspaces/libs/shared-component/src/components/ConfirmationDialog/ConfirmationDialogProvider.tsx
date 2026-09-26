import {
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
} from "@wso2/oxygen-ui";
import { createContext, useCallback, useContext, useMemo, useState } from "react";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";

export interface ConfirmationEvent {
  title: string;
  description: string;
  onConfirm: () => void;
  onCancel?: () => void;
  confirmButtonIcon?: React.ReactNode;
  confirmButtonColor?:
    | "primary"
    | "secondary"
    | "error"
    | "warning"
    | "info"
    | "success";
  confirmButtonText?: string;
  cancelButtonText?: string;
  /**
   * Optional analytics labels for this confirmation. Without them the
   * cancellation is still counted, just labelled from confirmButtonText — the
   * dialog's title and description are never reported, since they interpolate
   * resource names.
   */
  analytics?: {
    /** What is being acted on, e.g. "llm-provider", "agent", "pipeline". */
    entity: string;
    /** The action being confirmed, e.g. "delete", "undeploy". */
    action: string;
  };
}

export interface ConfirmationContextType {
  addConfirmation: (confirmation: ConfirmationEvent) => void;
}

const defaultContext: ConfirmationContextType = {
  addConfirmation: () => {},
};

export const ConfirmationDialogContext =
  createContext<ConfirmationContextType>(defaultContext);

export function useConfirmationDialog() {
  return useContext(ConfirmationDialogContext);
}

export function ConfirmationDialogProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [confirmations, setConfirmations] = useState<ConfirmationEvent[]>([]);
  const { track } = useTrack();
  const currentConfirmation = useMemo(() => {
    if (confirmations.length === 0) {
      return null;
    }
    return confirmations[confirmations.length - 1];
  }, [confirmations]);

  const handleConfirm = useCallback(() => {
    currentConfirmation?.onConfirm();
    setConfirmations((prev) => prev.slice(0, -1)); 
  }, [currentConfirmation]);

  const handleCancel = useCallback(() => {
    if (currentConfirmation) {
      // A backed-out destructive action leaves no server-side trace at all —
      // the whole point of tracking it here. Instrumented in the provider
      // rather than at each of the ~26 call sites so a new confirmation
      // cannot ship unmeasured.
      track(ConsoleAction.ConfirmationCancelled, {
        entity: currentConfirmation.analytics?.entity ?? "unspecified",
        destructive_action:
          currentConfirmation.analytics?.action ??
          currentConfirmation.confirmButtonText ??
          "unspecified",
      });
    }
    currentConfirmation?.onCancel?.();
    setConfirmations((prev) => prev.slice(0, -1));  
  }, [currentConfirmation, track]);

  const addConfirmation = useCallback(
    (confirmation: ConfirmationEvent) => {
      setConfirmations((prev) => [...prev, confirmation]);
    },
    []
  );

  return (
    <>
      <ConfirmationDialogContext.Provider value={{ addConfirmation }}>
        {children}
      </ConfirmationDialogContext.Provider>

      <Dialog open={confirmations.length > 0} onClose={handleCancel}>
        <DialogTitle>{currentConfirmation?.title}</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {currentConfirmation?.description}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancel}>
            {currentConfirmation?.cancelButtonText || "Cancel"}
          </Button>
          <Button
            onClick={handleConfirm}
            variant="contained"
            color={currentConfirmation?.confirmButtonColor || "primary"}
            startIcon={currentConfirmation?.confirmButtonIcon}
          >
            {currentConfirmation?.confirmButtonText || "Confirm"}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
