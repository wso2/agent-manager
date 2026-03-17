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
        const duration = message.duration || 6000;
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
            <div className="fixed bottom-4 right-4 pointer-events-none" style={{ zIndex: 1400 }}>
                {snackbars.map((snackbar, idx) => {
                    const {
                        id,
                        duration,
                        message,
                        type,
                        onClose,
                        ...snackbarProps
                    } = snackbar;
                    // Each snackbar is offset by 64px (height+gap) times its index
                    const verticalOffset = 64 + 64 * idx;
                    return (
                        <div
                            key={message}
                            className="pointer-events-auto"
                            style={{ zIndex: 1400 + idx }}
                        >
                            <Snackbar
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
                                style={{
                                    marginBottom: 8,
                                    position: 'absolute',
                                    bottom: verticalOffset,
                                    transition: 'bottom 0.3s ease, opacity 0.3s ease',
                                    zIndex: 1400 + idx,
                                }}
                            >
                                {type ? <Alert severity={type}>{message}</Alert> : undefined}
                            </Snackbar>
                        </div>
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
