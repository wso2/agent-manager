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
  Stack,
  TextField,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useCreateUser } from "@agent-management-platform/api-client";
import { TextInput } from "@agent-management-platform/views";
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
      navigate(usersPath);
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  };

  return (
    <>
      <BackButton to={usersPath} label="Back to Users" />
      <Stack spacing={3} sx={{ maxWidth: 700 }}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create user"}
          </Alert>
        )}

        {/* Account credentials */}
        <Form.Section>
          <Form.Header>Account Credentials</Form.Header>
          <Form.Stack spacing={2}>
            <FormControl fullWidth error={Boolean(errors.username)}>
              <FormLabel required>Username</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                fullWidth
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
            </FormControl>

            <TextInput
              maxLength={INPUT_LIMITS.PASSWORD}
              label="Password"
              required
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
          </Form.Stack>
        </Form.Section>

        {/* Profile */}
        <Form.Section>
          <Form.Header>Profile</Form.Header>
          <Form.Stack spacing={2}>
            <Box
              sx={{
                display: "grid",
                gap: 2,
                gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
              }}
            >
              <FormControl fullWidth>
                <FormLabel>First Name</FormLabel>
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                  fullWidth
                  value={givenName}
                  onChange={(e) => setGivenName(e.target.value)}
                  placeholder="John"
                />
              </FormControl>

              <FormControl fullWidth>
                <FormLabel>Last Name</FormLabel>
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                  fullWidth
                  value={familyName}
                  onChange={(e) => setFamilyName(e.target.value)}
                  placeholder="Doe"
                />
              </FormControl>
            </Box>
          </Form.Stack>
        </Form.Section>

        {/* Actions */}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={() => navigate(usersPath)}
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
