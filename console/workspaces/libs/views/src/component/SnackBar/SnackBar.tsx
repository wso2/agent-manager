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

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  useEffect,
} from 'react';
import {
  Alert,
  Box,
  Snackbar,
  SnackbarProps,
  Typography,
} from '@wso2/oxygen-ui';

type SnackBarType = 'error' | 'success' | 'info';

export interface SnackBarMessage
  extends Omit<SnackbarProps, 'open' | 'message' | 'children'> {
  id: string;
  message: string;
  duration?: number;
  type?: SnackBarType;
}

interface SnackBarContextType {
  pushSnackBar: (message: Omit<SnackBarMessage, 'id'>) => void;
}

const SnackBarContext = createContext<SnackBarContextType | undefined>(
  undefined
);

export const SnackBarProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [snackbars, setSnackbars] = useState<SnackBarMessage[]>([]);
  const timeoutsRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(
    new Map()
  );

  useEffect(() => {
    return () => {
      timeoutsRef.current.forEach((timeoutId) => clearTimeout(timeoutId));
      timeoutsRef.current.clear();
    };
  }, []);

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
      return [snackBarMessage, ...filtered];
    });

    if (duration > 0) {
      const timeoutId = setTimeout(() => {
        setSnackbars((prev) => prev.filter((sb) => sb.id !== id));
        timeoutsRef.current.delete(id);
      }, duration + 500);
      timeoutsRef.current.set(id, timeoutId);
    }
  }, []);

  const removeSnackBar = useCallback((id: string) => {
    const timeoutId = timeoutsRef.current.get(id);
    if (timeoutId) {
      clearTimeout(timeoutId);
      timeoutsRef.current.delete(id);
    }
    setSnackbars((prev) => prev.filter((sb) => sb.id !== id));
  }, []);

  return (
    <SnackBarContext.Provider value={{ pushSnackBar }}>
      {children}
      <Box
        sx={{
          position: 'fixed',
          bottom: 32,
          right: 0,
          zIndex: 1400,
          pointerEvents: 'none',
          display: 'flex',
          minWidth: 300,
        }}
      >
        {snackbars.map((snackbar, index) => {
          const { id, duration, message, type, onClose, ...snackbarProps } =
            snackbar;
          return (
            <Snackbar
              key={message} // Messages are unique
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
                marginBottom: 0,
                position: 'absolute',
                transition: 'transform 0.3s ease-in-out',
                transform: `translateY(-${index * 54}px)`,
              }}
            >
              {type ? (
                <Alert severity={type}>
                  <Typography noWrap variant="body2">
                    {message}
                  </Typography>
                </Alert>
              ) : undefined}
            </Snackbar>
          );
        })}
      </Box>
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
