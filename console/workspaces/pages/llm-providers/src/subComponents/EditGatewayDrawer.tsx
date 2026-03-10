/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Form,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useFormValidation,
} from "@agent-management-platform/views";
import { useUpdateGateway } from "@agent-management-platform/api-client";
import type { GatewayResponse, GatewayStatus, UpdateGatewayRequest } from "@agent-management-platform/types";
import { z } from "zod";

interface EditGatewayDrawerProps {
  open: boolean;
  onClose: () => void;
  gateway: GatewayResponse;
  orgId: string;
  onSuccess?: () => void;
}

interface EditGatewayFormValues {
  displayName: string;
  isCritical: boolean;
  status: GatewayStatus;
}

const editGatewaySchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(1, "Display name is required")
    .max(128, "Display name must be at most 128 characters"),
  isCritical: z.boolean(),
  status: z.enum(["ACTIVE", "INACTIVE", "PROVISIONING", "ERROR"]),
});

export function EditGatewayDrawer({
  open,
  onClose,
  gateway,
  orgId,
  onSuccess,
}: EditGatewayDrawerProps) {
  const [formData, setFormData] = useState<EditGatewayFormValues>({
    displayName: gateway.displayName,
    isCritical: gateway.isCritical,
    status: gateway.status,
  });

  const { errors, validateForm, clearErrors, setFieldError, validateField } =
    useFormValidation<EditGatewayFormValues>(editGatewaySchema);

  const { mutate: updateGateway, isPending, error: updateError } = useUpdateGateway();

  useEffect(() => {
    if (open) {
      setFormData({
        displayName: gateway.displayName,
        isCritical: gateway.isCritical,
        status: gateway.status,
      });
      clearErrors();
    }
  }, [gateway, open, clearErrors]);

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!validateForm(formData)) return;

      const payload: UpdateGatewayRequest = {
        displayName: formData.displayName.trim(),
        isCritical: formData.isCritical,
        status: formData.status,
      };

      updateGateway(
        {
          params: { orgName: orgId, gatewayId: gateway.uuid },
          body: payload,
        },
        {
          onSuccess: () => {
            clearErrors();
            onClose();
            onSuccess?.();
          },
        }
      );
    },
    [formData, validateForm, updateGateway, orgId, gateway.uuid, onClose, onSuccess, clearErrors]
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit AI Gateway"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            {updateError && (
              <Alert severity="error" sx={{ mb: 1 }}>
                {updateError instanceof Error
                  ? updateError.message
                  : "Failed to update gateway."}
              </Alert>
            )}
            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <Typography variant="subtitle1" fontWeight={600}>
                  Gateway Details
                </Typography>
                <Form.ElementWrapper label="Display Name" name="displayName">
                  <TextField
                    id="displayName"
                    placeholder="e.g., Production AI Gateway"
                    size="small"
                    fullWidth
                    disabled={isPending}
                    value={formData.displayName}
                    onChange={(e) => {
                      const v = e.target.value;
                      setFormData((prev) => ({ ...prev, displayName: v }));
                      setFieldError("displayName", validateField("displayName", v, { ...formData, displayName: v }));
                    }}
                    error={!!errors.displayName}
                    helperText={errors.displayName}
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Critical production gateway" name="isCritical">
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Switch
                      checked={formData.isCritical}
                      onChange={(_, checked) =>
                        setFormData((prev) => ({ ...prev, isCritical: checked }))
                      }
                      disabled={isPending}
                    />
                    <Typography variant="caption" color="text.secondary">
                      Mark as critical for production deployments
                    </Typography>
                  </Stack>
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Status" name="status">
                  <Stack direction="row" spacing={1} flexWrap="wrap">
                    {(["ACTIVE", "INACTIVE"] as const).map((s) => (
                      <Button
                        key={s}
                        size="small"
                        variant={formData.status === s ? "contained" : "outlined"}
                        onClick={() =>
                          setFormData((prev) => ({ ...prev, status: s }))
                        }
                        disabled={isPending}
                      >
                        {s}
                      </Button>
                    ))}
                  </Stack>
                </Form.ElementWrapper>
              </CardContent>
            </Card>
            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isPending}
              >
                {isPending ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
