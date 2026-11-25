import {
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Form,
  OutlinedInput,
} from "@wso2/oxygen-ui";
import {
  createContext,
  useCallback,
  useContext,
  useId,
  useMemo,
  useState,
  type FormEvent,
} from "react";
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
   * When set, the user types this exact text (e.g. the resource's name) before
   * Confirm is enabled.
   */
  confirmationText?: string;
  /** The label above the type-to-confirm field. Default: `Type "<text>" to confirm`. */
  confirmationInputLabel?: string;
  /**
   * Shows Confirm disabled: the action can't be taken (for example, a resource in use can't be
   * deleted) and the description says why.
   */
  confirmDisabled?: boolean;
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
  // The typed text belongs to the confirmation it was typed for, so the next one starts empty.
  const [typed, setTyped] = useState<{ for: ConfirmationEvent | null; value: string }>({
    for: null,
    value: "",
  });
  const typedValue = typed.for === currentConfirmation ? typed.value : "";
  const confirmationText = currentConfirmation?.confirmationText;
  const canConfirm =
    !currentConfirmation?.confirmDisabled && (!confirmationText || typedValue === confirmationText);
  const confirmationFieldId = useId();

  const handleConfirm = useCallback(() => {
    if (!canConfirm) return;
    currentConfirmation?.onConfirm();
    setConfirmations((prev) => prev.slice(0, -1));
  }, [canConfirm, currentConfirmation]);

  // A form, so Enter in the type-to-confirm field confirms.
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    handleConfirm();
  };

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
        <form noValidate onSubmit={handleSubmit}>
          <DialogContent>
            <Form.Stack spacing={2}>
              <DialogContentText>
                {currentConfirmation?.description}
              </DialogContentText>
              {confirmationText && !currentConfirmation?.confirmDisabled && (
                <Form.ElementWrapper
                  label={
                    currentConfirmation?.confirmationInputLabel ??
                    `Type "${confirmationText}" to confirm`
                  }
                  name={confirmationFieldId}
                >
                  <OutlinedInput
                    autoFocus
                    id={confirmationFieldId}
                    onChange={(event) =>
                      setTyped({ for: currentConfirmation, value: event.target.value })
                    }
                    placeholder={confirmationText}
                    size="small"
                    value={typedValue}
                  />
                </Form.ElementWrapper>
              )}
            </Form.Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCancel}>
              {currentConfirmation?.cancelButtonText || "Cancel"}
            </Button>
            <Button
              disabled={!canConfirm}
              type="submit"
              variant="contained"
              color={currentConfirmation?.confirmButtonColor || "primary"}
              startIcon={currentConfirmation?.confirmButtonIcon}
            >
              {currentConfirmation?.confirmButtonText || "Confirm"}
            </Button>
          </DialogActions>
        </form>
      </Dialog>
    </>
  );
}
