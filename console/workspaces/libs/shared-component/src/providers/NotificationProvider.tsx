/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useMemo, useState } from 'react';
import { Alert, Snackbar } from '@wso2/oxygen-ui';
import { NotificationContext, NotificationType } from './NotificationContext';

interface Notification {
  id: string;
  type: NotificationType;
  message: string;
}

interface NotificationProviderProps {
  children: React.ReactNode;
}

const AUTO_HIDE_DURATION = 5000;

export function NotificationProvider({ children }: NotificationProviderProps) {
  const [notifications, setNotifications] = useState<Notification[]>([]);

  // Add a new notification
  const notify = useCallback((type: NotificationType, message: string) => {
    const id = Date.now().toString();
    setNotifications((prev) => [...prev, { id, type, message }]);
  }, []);

  // Remove a notification by id
  const removeNotification = useCallback((id: string) => {
    setNotifications((prev) => prev.filter((n) => n.id !== id));
  }, []);

  // Memoize context value to prevent unnecessary re-renders
  const contextValue = useMemo(() => ({ notify }), [notify]);

  return (
    <NotificationContext.Provider value={contextValue}>
      {children}
      {/* Render notifications - show only the first one, queue the rest */}
      {notifications.length > 0 && (
        <Snackbar
          open
          autoHideDuration={AUTO_HIDE_DURATION}
          onClose={() => removeNotification(notifications[0].id)}
          anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        >
          <Alert
            severity={notifications[0].type}
            onClose={() => removeNotification(notifications[0].id)}
            sx={{ width: '100%' }}
          >
            {notifications[0].message}
          </Alert>
        </Snackbar>
      )}
    </NotificationContext.Provider>
  );
}
