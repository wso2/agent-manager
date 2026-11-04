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

import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useCreateSelfHostedGateway,
  useCreateSelfHostedGatewayToken,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import type { GatewayResponse } from "@agent-management-platform/types";
import {
  addSelfHostedGatewaySchema,
  GATEWAY_DISPLAY_NAME_MAX_LENGTH,
  type AddSelfHostedGatewayFormValues,
} from "../form/schema";
import { SelfHostedSetupSteps } from "./SelfHostedSetupSteps";

interface AddSelfHostedGatewayDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
  /** Pre-fills the form; used when the drawer is opened from another page. */
  presetEnvironment?: string;
  presetDisplayName?: string;
  presetDescription?: string;
}

interface CreatedGateway {
  gateway: GatewayResponse;
  /** Null when minting failed; the setup steps then offer Reconfigure. */
  token: string | null;
  tokenError?: string;
}

export function AddSelfHostedGatewayDrawer({
  open,
  onClose,
  orgId,
  presetEnvironment = "",
  presetDisplayName = "",
  presetDescription = "",
}: AddSelfHostedGatewayDrawerProps) {
  const [values, setValues] = useState<AddSelfHostedGatewayFormValues>({
    displayName: presetDisplayName,
    environment: presetEnvironment,
    publicUrl: "",
    description: presetDescription,
  });
  const { errors, validateForm, clearErrors } =
    useFormValidation<AddSelfHostedGatewayFormValues>(addSelfHostedGatewaySchema);
  // Errors show from the first submit on, then follow every edit.
  const [submitted, setSubmitted] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedGateway | null>(null);

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { mutateAsync: createGateway, isPending: creating } = useCreateSelfHostedGateway();
  const { mutateAsync: createToken, isPending: minting } = useCreateSelfHostedGatewayToken();
  const isPending = creating || minting;

  useEffect(() => {
    if (!open) {
      setCreated(null);
      return;
    }
    setValues({
      displayName: presetDisplayName,
      environment: presetEnvironment,
      publicUrl: "",
      description: presetDescription,
    });
    setSubmitted(false);
    clearErrors();
    setSubmitError(null);
    setCreated(null);
  }, [open, presetDisplayName, presetEnvironment, presetDescription, clearErrors]);

  const setField = (field: keyof AddSelfHostedGatewayFormValues, value: string) => {
    const next = { ...values, [field]: value };
    setValues(next);
    if (submitted) validateForm(next);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (!validateForm(values)) return;
    setSubmitError(null);
    const { displayName, environment, publicUrl, description } = values;

    let gateway: GatewayResponse;
    try {
      gateway = await createGateway({
        params: { orgName: orgId },
        body: {
          displayName: displayName.trim(),
          description: description.trim() || undefined,
          environment,
          endpoints: [publicUrl.trim()],
          functionalityType: "ai",
        },
      });
    } catch (err) {
      setSubmitError(getErrorMessage(err) || "Failed to create the gateway.");
      return;
    }

    let token: string | null = null;
    let tokenError: string | undefined;
    try {
      token = (await createToken({ orgName: orgId, gatewayId: gateway.uuid })).token;
    } catch (err) {
      tokenError = getErrorMessage(err) || undefined;
    }
    setCreated({ gateway, token, tokenError });
  };

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Plus size={24} />}
        title={created ? `Set up ${created.gateway.displayName}` : "Add Gateway"}
        onClose={onClose}
      />
      <DrawerContent>
        {created ? (
          <Stack spacing={2}>
            {created.token ? (
              <Alert severity="success">
                Gateway created. Run the steps below on your own infrastructure to connect it.
              </Alert>
            ) : (
              <Alert severity="warning">
                Gateway created, but its token could not be generated.
                {created.tokenError ? ` ${created.tokenError}.` : ""} Use Reconfigure to generate one.
              </Alert>
            )}
            <SelfHostedSetupSteps
              orgName={orgId}
              gatewayId={created.gateway.uuid}
              initialToken={created.token}
            />
            <Box display="flex" justifyContent="flex-end">
              <Button variant="contained" onClick={onClose}>
                Done
              </Button>
            </Box>
          </Stack>
        ) : (
          <form onSubmit={handleSubmit}>
            <Stack spacing={3}>
              {submitError && (
                <Alert severity="error">
                  <Typography variant="body2">{submitError}</Typography>
                </Alert>
              )}
              <Form.Section>
                <Form.Header>Gateway Details</Form.Header>
                <Form.Stack spacing={2}>
                  <FormControl fullWidth error={Boolean(errors.displayName)}>
                    <FormLabel required>Display Name</FormLabel>
                    <TextField
                      slotProps={{ htmlInput: { maxLength: GATEWAY_DISPLAY_NAME_MAX_LENGTH } }}
                      fullWidth
                      size="small"
                      value={values.displayName}
                      onChange={(e) => setField("displayName", e.target.value)}
                      placeholder="e.g., Production Gateway"
                      error={Boolean(errors.displayName)}
                      helperText={errors.displayName}
                      disabled={isPending}
                    />
                  </FormControl>
                  <FormControl fullWidth error={Boolean(errors.environment)}>
                    <FormLabel required>Environment</FormLabel>
                    <Select
                      size="small"
                      value={values.environment}
                      onChange={(e) => setField("environment", String(e.target.value))}
                      displayEmpty
                      disabled={isPending}
                      inputProps={{ "aria-label": "Environment" }}
                    >
                      {(environments ?? []).map((env) => (
                        <MenuItem key={env.name} value={env.name}>
                          {env.displayName || env.name}
                        </MenuItem>
                      ))}
                    </Select>
                    {errors.environment && (
                      <Typography variant="caption" color="error">
                        {errors.environment}
                      </Typography>
                    )}
                  </FormControl>
                  <FormControl fullWidth error={Boolean(errors.publicUrl)}>
                    <FormLabel required>Public URL</FormLabel>
                    <TextField
                      fullWidth
                      size="small"
                      value={values.publicUrl}
                      onChange={(e) => setField("publicUrl", e.target.value)}
                      placeholder="https://gateway.example.com"
                      error={Boolean(errors.publicUrl)}
                      helperText={
                        errors.publicUrl ??
                        "The address clients use to reach this gateway."
                      }
                      disabled={isPending}
                    />
                  </FormControl>
                  <FormControl fullWidth>
                    <FormLabel>Description</FormLabel>
                    <TextField
                      fullWidth
                      size="small"
                      multiline
                      minRows={2}
                      value={values.description}
                      onChange={(e) => setField("description", e.target.value)}
                      disabled={isPending}
                    />
                  </FormControl>
                </Form.Stack>
              </Form.Section>
              <Box display="flex" justifyContent="flex-end" gap={1}>
                <Button variant="outlined" color="inherit" onClick={onClose} disabled={isPending}>
                  Cancel
                </Button>
                <Button type="submit" variant="contained" disabled={isPending}>
                  {isPending ? "Creating..." : "Create"}
                </Button>
              </Box>
            </Stack>
          </form>
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}
