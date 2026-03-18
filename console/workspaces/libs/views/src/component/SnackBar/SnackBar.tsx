import React, { createContext, useContext, useState, useCallback } from 'react';
import { Alert, Snackbar, SnackbarProps } from '@wso2/oxygen-ui';

type SnackBarType = 'error' | 'success' | 'info';

export interface SnackBarMessage extends Omit<SnackbarProps, 'open' | 'message' | 'children'> {
    id: string;
    message: string;
    duration?: number;
    type?: SnackBarType;
}

interface SnackBarContextType {
    pushSnackBar: (message: Omit<SnackBarMessage, 'id'>) => void;
}

const SnackBarContext = createContext<SnackBarContextType | undefined>(undefined);

export const SnackBarProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [snackbars, setSnackbars] = useState<SnackBarMessage[]>([]);

    const pushSnackBar = useCallback((message: Omit<SnackBarMessage, 'id'>) => {
        const id = Date.now().toString();
        const duration = message.duration ?? 6000;
        const snackBarMessage: SnackBarMessage = {
            ...message,
            id,
            duration,
        };

        setSnackbars((prev) => {
            // Remove any snackbar with the same message
            let filtered = prev.filter((sb) => sb.message !== message.message);
            // Enforce max 3 snackbars: remove oldest if needed
            if (filtered.length >= 3) {
                filtered = filtered.slice(filtered.length - 2);
            }
            return [...filtered, snackBarMessage];
        });

        if (duration > 0) {
            setTimeout(() => {
                setSnackbars((prev) => prev.filter((sb) => sb.id !== id));
            }, duration);
        }
    }, []);

    const removeSnackBar = (id: string) => {
        setSnackbars((prev) => prev.filter((sb) => sb.id !== id));
    };

    return (
        <SnackBarContext.Provider value={{ pushSnackBar }}>
            {children}
            <div
                style={{ position: 'fixed', bottom: 32, right: 32, zIndex: 1400, pointerEvents: 'none', display: 'flex', flexDirection: 'column', gap: 8 }}
            >
                {snackbars.map((snackbar) => {
                    const {
                        id,
                        duration,
                        message,
                        type,
                        onClose,
                        ...snackbarProps
                    } = snackbar;
                    return (
                        <Snackbar
                            key={message}
                            open={true}
                            anchorOrigin={{
                                vertical: 'bottom',
                                horizontal: 'right',
                            }}
                            autoHideDuration={duration}
                            onClose={(event, reason) => {
                                onClose?.(event, reason);
                                removeSnackBar(id);
                            }}
                            message={type ? undefined : message}
                            {...snackbarProps}
                            style={{ marginBottom: 0, position: 'static' }}
                        >
                            {type ? <Alert severity={type}>{message}</Alert> : undefined}
                        </Snackbar>
                    );
                })}
            </div>
        </SnackBarContext.Provider>
    );
};

export const useSnackBar = (): SnackBarContextType => {
    const context = useContext(SnackBarContext);
    if (!context) {
        throw new Error('useSnackBar must be used within SnackBarProvider');
    }
    return context;
};
