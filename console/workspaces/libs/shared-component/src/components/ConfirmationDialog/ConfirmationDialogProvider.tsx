import {
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
} from "@wso2/oxygen-ui";
import { createContext, useCallback, useContext, useMemo, useState } from "react";

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
  const currentConfirmation = useMemo(() => {
    if (confirmations.length === 0) {
      return null;
    }
    return confirmations[confirmations.length - 1];
  }, [confirmations]);

  const handleConfirm = useCallback(() => {
    currentConfirmation?.onConfirm();
    setConfirmations(confirmations.slice(0, -1));
  }, [currentConfirmation, confirmations]);

  const handleCancel = useCallback(() => {
    currentConfirmation?.onCancel?.();
    setConfirmations(confirmations.slice(0, -1));
  }, [currentConfirmation, confirmations]);

  const addConfirmation = useCallback(
    (confirmation: ConfirmationEvent) => {
      setConfirmations((prev) => [...prev, confirmation]);
    },
    [setConfirmations]
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
          <Button onClick={handleCancel}>Cancel</Button>
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
