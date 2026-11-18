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

import React, { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  Grid,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  PageLayout,
  useFormValidation,
  useReturnTo,
} from "@agent-management-platform/views";
import {
  useCreateSelfHostedGateway,
  useCreateSelfHostedGatewayToken,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  onlyEnvironmentName,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type GatewayResponse } from "@agent-management-platform/types";
import {
  addSelfHostedGatewaySchema,
  deriveGatewayName,
  GATEWAY_DISPLAY_NAME_MAX_LENGTH,
  GATEWAY_NAME_MAX_LENGTH,
  type AddSelfHostedGatewayFormValues,
} from "../form/schema";
import { keepNewGatewayToken } from "./newGatewayToken";
import type { SetupGatewayState } from "./SetupGateway";

const EMPTY_FORM: AddSelfHostedGatewayFormValues = {
  displayName: "",
  name: "",
  environment: "",
  publicUrl: "",
  description: "",
};

/**
 * Create page for a self-hosted gateway. Once the gateway and its registration token
 * are created, it moves on to the setup page, leaving the token there in memory.
 */
export function AddGateway() {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const listHref = generatePath(absoluteRouteMap.children.org.children.gateways.path, { orgId });
  const returnTo = useReturnTo(listHref);

  const [values, setValues] = useState<AddSelfHostedGatewayFormValues>(EMPTY_FORM);
  const { errors, validateForm } =
    useFormValidation<AddSelfHostedGatewayFormValues>(addSelfHostedGatewaySchema);
  // Errors show from the first submit on, then follow every edit.
  const [submitted, setSubmitted] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { mutateAsync: createGateway, isPending: creating } = useCreateSelfHostedGateway();
  const { mutateAsync: createToken, isPending: minting } = useCreateSelfHostedGatewayToken();
  const isPending = creating || minting;

  // The environment the form fills in by itself is not a user change.
  const isDirty = (Object.keys(EMPTY_FORM) as (keyof AddSelfHostedGatewayFormValues)[]).some(
      (key) => values[key] !== EMPTY_FORM[key],
    );
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  // With a single environment there is nothing to choose: the field is hidden
  // and the form uses that environment (see onlyEnvironmentName).
  const onlyEnvironment = onlyEnvironmentName(environments);
  const withEnvironment = (form: AddSelfHostedGatewayFormValues) =>
    form.environment || !onlyEnvironment ? form : { ...form, environment: onlyEnvironment };

  const update = (next: AddSelfHostedGatewayFormValues) => {
    setValues(next);
    if (submitted) validateForm(withEnvironment(next));
  };
  const setField = (field: keyof AddSelfHostedGatewayFormValues, value: string) =>
    update({ ...values, [field]: value });

  // The name follows the display name until the user edits it.
  const setDisplayName = (displayName: string) => {
    const nameFollowsDisplayName =
      values.name === "" || values.name === deriveGatewayName(values.displayName);
    update({
      ...values,
      displayName,
      name: nameFollowsDisplayName ? deriveGatewayName(displayName) : values.name,
    });
  };

  const handleCancel = () => allowNavigation(() => navigate(returnTo));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    const submittedValues = withEnvironment(values);
    if (!validateForm(submittedValues)) return;
    setSubmitError(null);

    const { displayName, name, environment, publicUrl, description } = submittedValues;
    let gateway: GatewayResponse;
    try {
      gateway = await createGateway({
        params: { orgName: orgId },
        body: {
          displayName: displayName.trim(),
          handle: name.trim(),
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

    const state: SetupGatewayState = {};
    try {
      const { token } = await createToken({ orgName: orgId, gatewayId: gateway.uuid });
      keepNewGatewayToken(gateway.uuid, token);
    } catch (err) {
      state.tokenFailed = true;
      state.tokenError = getErrorMessage(err) || undefined;
    }
    // Replace: the gateway now exists, so Back must not return to this form.
    allowNavigation(() =>
      navigate(
        generatePath(absoluteRouteMap.children.org.children.gateways.children.setup.path, {
          orgId,
          gatewayId: gateway.uuid,
        }),
        { replace: true, state },
      ),
    );
  };

  return (
    <PageLayout title="Add gateway" disableIcon backHref={returnTo} backLabel="Back">
      <form onSubmit={handleSubmit}>
        <Stack spacing={3}>
          {submitError && (
            <Alert severity="error">
              <Typography variant="body2">{submitError}</Typography>
            </Alert>
          )}
          <Grid container>
            <Grid size={{ xs: 12, md: 8, xl: 6 }}>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: GATEWAY_DISPLAY_NAME_MAX_LENGTH } }}
                    fullWidth
                    size="small"
                    value={values.displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    placeholder="e.g., Production Gateway"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isPending}
                  />
                </FormControl>
                <FormControl fullWidth error={Boolean(errors.name)}>
                  <FormLabel required>Name</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: GATEWAY_NAME_MAX_LENGTH } }}
                    fullWidth
                    size="small"
                    value={values.name}
                    onChange={(e) => setField("name", e.target.value)}
                    placeholder="e.g., production-gateway"
                    error={Boolean(errors.name)}
                    helperText={
                      errors.name ??
                      "The resource name used by the API. Unique in the organization; it can't be changed later."
                    }
                    disabled={isPending}
                  />
                </FormControl>
                {!onlyEnvironment && (
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
                )}
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
                      errors.publicUrl ?? "The address clients use to reach this gateway."
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
            </Grid>
          </Grid>
          <Box display="flex" justifyContent="flex-start" gap={1}>
            <Button variant="outlined" color="inherit" onClick={handleCancel} disabled={isPending}>
              Cancel
            </Button>
            <Button type="submit" variant="contained" disabled={isPending}>
              {isPending ? "Creating..." : "Create gateway"}
            </Button>
          </Box>
        </Stack>
      </form>
    </PageLayout>
  );
}

export default AddGateway;
