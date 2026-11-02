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
  Button,
  Form,
  Grid,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useCreateUser } from "@agent-management-platform/api-client";
import { TextInput } from "@agent-management-platform/views";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import { BackButton } from "./components/BackButton";

export const UserCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [givenName, setGivenName] = useState("");
  const [familyName, setFamilyName] = useState("");
  const [errors, setErrors] = useState<{
    username?: string;
    password?: string;
  }>({});

  const isDirty = !!(username || password || givenName || familyName);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const {
    mutateAsync: createUser,
    isPending: isCreating,
    error: createError,
  } = useCreateUser();

  const usersPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.users.path,
        { orgId },
      )
    : "#";

  const validate = (): boolean => {
    const next: typeof errors = {};
    if (!username.trim()) next.username = "Username is required";
    if (!password.trim()) next.password = "Password is required";
    setErrors(next);
    return Object.keys(next).length === 0;
  };

  const handleSubmit = async () => {
    if (!validate()) return;

    const attributes: Record<string, string> = {
      username: username.trim(),
      password,
    };

    if (givenName.trim()) {
      attributes.given_name = givenName.trim();
    }
    if (familyName.trim()) {
      attributes.family_name = familyName.trim();
    }

    try {
      await createUser({
        params: { orgName: orgId },
        body: {
          type: "engineer",
          attributes,
        },
      });
      allowNavigation(() => navigate(usersPath));
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  };

  return (
    <>
      <BackButton to={usersPath} label="Back to users" />
      <Stack spacing={3} sx={{ maxWidth: 700 }}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create user"}
          </Alert>
        )}

        <Form.Stack spacing={3}>
          {/* Account credentials */}
          <Typography variant="subtitle1">Account Credentials</Typography>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Username"
                maxLength={INPUT_LIMITS.NAME}
                hideCharacterCount={username.length < INPUT_LIMITS.NAME}
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value);
                  if (errors.username)
                    setErrors((p) => ({ ...p, username: undefined }));
                }}
                placeholder="john.doe"
                autoComplete="off"
                error={Boolean(errors.username)}
                helperText={errors.username}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                maxLength={INPUT_LIMITS.PASSWORD}
                hideCharacterCount={password.length < INPUT_LIMITS.PASSWORD}
                label="Password"
                type="password"
                showPasswordToggle
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  if (errors.password)
                    setErrors((p) => ({ ...p, password: undefined }));
                }}
                autoComplete="new-password"
                error={Boolean(errors.password)}
                helperText={errors.password}
              />
            </Grid>
            <Grid size={{ xl: 4 }} sx={{ display: { xs: "none", xl: "block" } }} />
          </Grid>

          {/* Profile */}
          <Typography variant="subtitle1">Profile</Typography>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="First Name (optional)"
                maxLength={INPUT_LIMITS.NAME}
                hideCharacterCount={givenName.length < INPUT_LIMITS.NAME}
                value={givenName}
                onChange={(e) => setGivenName(e.target.value)}
                placeholder="John"
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Last Name (optional)"
                maxLength={INPUT_LIMITS.NAME}
                hideCharacterCount={familyName.length < INPUT_LIMITS.NAME}
                value={familyName}
                onChange={(e) => setFamilyName(e.target.value)}
                placeholder="Doe"
              />
            </Grid>
          </Grid>
        </Form.Stack>

        {/* Actions */}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={() => allowNavigation(() => navigate(usersPath))}
            disabled={isCreating}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSubmit}
            disabled={isCreating || !username.trim() || !password.trim()}
          >
            {isCreating ? "Creating..." : "Create User"}
          </Button>
        </Stack>
      </Stack>
    </>
  );
};
