/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { type ReactNode } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { Alert, Button, Stack } from "@wso2/oxygen-ui";
import { AlertTriangle, BookOpen } from "@wso2/oxygen-ui-icons-react";
import {
  EnvironmentVariablesReference,
  type EnvVarReferenceRow,
} from "@agent-management-platform/shared-component";

interface EnvironmentVariablesGuideDrawerProps {
  open: boolean;
  title?: string;
  description: ReactNode;
  rows: EnvVarReferenceRow[];
  isDirty?: boolean;
  isSaving?: boolean;
  hasInvalidNames?: boolean;
  /** When set, save is disabled and this reason is shown above the actions. */
  saveBlockedReason?: string;
  error?: unknown;
  onClose: () => void;
  onCancel?: () => void;
  onNameChange?: (key: string, value: string) => void;
  onSave?: () => void;
  children?: ReactNode;
}

export function EnvironmentVariablesGuideDrawer({
  open,
  title = "Environment Variables & Integration Guide",
  description,
  rows,
  isDirty = false,
  isSaving = false,
  hasInvalidNames = false,
  saveBlockedReason,
  error,
  onClose,
  onCancel,
  onNameChange,
  onSave,
  children,
}: EnvironmentVariablesGuideDrawerProps) {
  const errorMessage = error
    ? error instanceof Error
      ? error.message
      : "Failed to update configuration. Please try again."
    : undefined;

  return (
    <DrawerWrapper
      open={open}
      onClose={() => onClose()}
      minWidth={640}
      maxWidth={640}
    >
      <DrawerHeader
        icon={<BookOpen size={24} />}
        title={title}
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={3}>
          {errorMessage && (
            <Alert severity="error" icon={<AlertTriangle size={18} />}>
              {errorMessage}
            </Alert>
          )}

          {saveBlockedReason && (
            <Alert severity="warning">{saveBlockedReason}</Alert>
          )}

          {isDirty && !errorMessage && (
            <Alert
              severity="warning"
              action={
                <Stack direction="row" spacing={1} alignItems="center">
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={onCancel ?? onClose}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="small"
                    variant="contained"
                    onClick={onSave}
                    disabled={isSaving || hasInvalidNames || !!saveBlockedReason}
                  >
                    {isSaving ? "Saving..." : "Save changes"}
                  </Button>
                </Stack>
              }
            >
              You have unsaved changes.
            </Alert>
          )}

          <EnvironmentVariablesReference
            variant="plain"
            description={description}
            rows={rows}
            onNameChange={onNameChange}
          >
            {children}
          </EnvironmentVariablesReference>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
