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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Stack,
  Tabs,
  Tab,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { TextInput } from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useUpdateUserProfile, useGetUserProfile } from "@agent-management-platform/api-client";
import { globalConfig, INPUT_LIMITS } from "@agent-management-platform/types";

type ActiveTab = "profile" | "password";

export const ProfilePage: React.FC = () => {
  const theme = useTheme();
  const { userInfo } = useAuthHooks();
  const { orgId } = useParams<{ orgId: string }>();
  const [activeTab, setActiveTab] = useState<ActiveTab>("profile");
  const isProfileManagementEnabled = globalConfig.featureFlags?.enableProfileManagement === true;

  const [profileData, setProfileData] = useState({
    username: userInfo?.username || "",
    given_name: userInfo?.givenName || "",
    family_name: userInfo?.familyName || "",
    email: userInfo?.email || "",
  });
  const [profileErrors, setProfileErrors] = useState<Record<string, string>>({});
  const {
    mutateAsync: updateProfile,
    isPending: isUpdatingProfile,
    error: profileError,
  } = useUpdateUserProfile();

  const [credentialData, setCredentialData] = useState({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [credentialErrors, setCredentialErrors] = useState<Record<string, string>>({});

  const [successMessage, setSuccessMessage] = useState("");

  const getErrorMessage = (error: unknown): string => {
    if (error && typeof error === "object" && "message" in error) {
      return (error as Error).message;
    }
    return "An error occurred";
  };

  const { data: userProfile } = useGetUserProfile({
    orgName: orgId || "default",
    userId: userInfo?.sub || "",
  });

  useEffect(() => {
    if (userProfile?.attributes) {
      setProfileData({
        username: (userProfile.attributes.username as string) || "",
        given_name: (userProfile.attributes.given_name as string) || "",
        family_name: (userProfile.attributes.family_name as string) || "",
        email: (userProfile.attributes.email as string) || "",
      });
    }
  }, [userProfile]);

  const handleTabChange = (_event: React.SyntheticEvent, newValue: ActiveTab) => {
    setActiveTab(newValue);
    setSuccessMessage("");
  };

  const validateProfile = (): boolean => {
    const errors: Record<string, string> = {};
    if (!profileData.username.trim()) {
      errors.username = "Username is required";
    }
    if (!profileData.email.trim()) {
      errors.email = "Email is required";
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(profileData.email)) {
      errors.email = "Invalid email format";
    }
    setProfileErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const validateCredentials = (): boolean => {
    const errors: Record<string, string> = {};
    if (!credentialData.newPassword.trim()) {
      errors.newPassword = "New password is required";
    } else if (credentialData.newPassword.length < 8) {
      errors.newPassword = "Password must be at least 8 characters";
    }
    if (credentialData.newPassword !== credentialData.confirmPassword) {
      errors.confirmPassword = "Passwords do not match";
    }
    setCredentialErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleProfileSubmit = async () => {
    if (!validateProfile() || !orgId || !userInfo?.sub) return;

    try {
      await updateProfile({
        params: { orgName: orgId, userId: userInfo.sub },
        body: {
          attributes: {
            username: profileData.username.trim(),
            given_name: profileData.given_name.trim(),
            family_name: profileData.family_name.trim(),
            email: profileData.email.trim(),
          },
        },
      });
      setSuccessMessage("Profile updated successfully");
      setProfileErrors({});
    } catch {
      // Error is shown via profileError state
    }
  };

  const handleCredentialSubmit = async () => {
    if (!validateCredentials() || !orgId || !userInfo?.sub) return;

    try {
      await updateProfile({
        params: { orgName: orgId, userId: userInfo.sub },
        body: {
          attributes: {
            password: credentialData.newPassword,
          },
        },
      });
      setSuccessMessage("Password updated successfully");
      setCredentialData({
        currentPassword: "",
        newPassword: "",
        confirmPassword: "",
      });
      setCredentialErrors({});
    } catch {
      // Error is shown via profileError state
    }
  };

  return (
    <Stack spacing={2}>
      {successMessage && (
        <Alert severity="success" onClose={() => setSuccessMessage("")}>
          {successMessage}
        </Alert>
      )}

      {!!profileError && (
        <Alert severity="error">
          {getErrorMessage(profileError)}
        </Alert>
      )}

      <Tabs
        value={activeTab}
        onChange={handleTabChange}
        sx={{ borderBottom: `1px solid ${theme.palette.divider}` }}
      >
        <Tab label="Profile Information" value="profile" />
        <Tab label="Change Password" value="password" disabled={!isProfileManagementEnabled} />
      </Tabs>

      {activeTab === "profile" && (
        <Stack spacing={2}>
          <Stack spacing={1.5}>
            <Typography variant="subtitle2" color="text.secondary">Account Information</Typography>
            <Stack spacing={2}>
              <TextInput
                maxLength={INPUT_LIMITS.NAME}
                label="Username"
                required
                disabled={!isProfileManagementEnabled}
                value={profileData.username}
                onChange={(e) => {
                  setProfileData({ ...profileData, username: e.target.value });
                  if (profileErrors.username) {
                    setProfileErrors({ ...profileErrors, username: "" });
                  }
                }}
                error={Boolean(profileErrors.username)}
                helperText={profileErrors.username}
              />
              <TextInput
                maxLength={INPUT_LIMITS.SHORT_TEXT}
                label="Email"
                type="email"
                required
                disabled={!isProfileManagementEnabled}
                value={profileData.email}
                onChange={(e) => {
                  setProfileData({ ...profileData, email: e.target.value });
                  if (profileErrors.email) {
                    setProfileErrors({ ...profileErrors, email: "" });
                  }
                }}
                error={Boolean(profileErrors.email)}
                helperText={profileErrors.email}
              />
            </Stack>
          </Stack>

          <Stack spacing={1.5}>
            <Typography variant="subtitle2" color="text.secondary">Personal Information</Typography>
            <Box sx={{ display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" } }}>
              <TextInput
                maxLength={INPUT_LIMITS.NAME}
                label="First Name"
                disabled={!isProfileManagementEnabled}
                value={profileData.given_name}
                onChange={(e) => setProfileData({ ...profileData, given_name: e.target.value })}
              />
              <TextInput
                maxLength={INPUT_LIMITS.NAME}
                label="Last Name"
                disabled={!isProfileManagementEnabled}
                value={profileData.family_name}
                onChange={(e) => setProfileData({ ...profileData, family_name: e.target.value })}
              />
            </Box>
          </Stack>

          {isProfileManagementEnabled && (
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="contained" onClick={handleProfileSubmit} disabled={isUpdatingProfile}>
                {isUpdatingProfile ? "Saving..." : "Save Changes"}
              </Button>
            </Stack>
          )}
        </Stack>
      )}

      {activeTab === "password" && (
        <Stack spacing={2}>
          <Stack spacing={1.5}>
            <Typography variant="subtitle2" color="text.secondary">Update Password</Typography>
            <Typography variant="body2" color="text.secondary">
              Enter a new password to update your account security.
            </Typography>
            <Stack spacing={2}>
              <TextInput
                maxLength={INPUT_LIMITS.PASSWORD}
                label="New Password"
                type="password"
                showPasswordToggle
                required
                value={credentialData.newPassword}
                onChange={(e) => {
                  setCredentialData({ ...credentialData, newPassword: e.target.value });
                  if (credentialErrors.newPassword) {
                    setCredentialErrors({ ...credentialErrors, newPassword: "" });
                  }
                }}
                error={Boolean(credentialErrors.newPassword)}
                helperText={credentialErrors.newPassword || "Minimum 8 characters"}
              />
              <TextInput
                maxLength={INPUT_LIMITS.PASSWORD}
                label="Confirm Password"
                type="password"
                showPasswordToggle
                required
                value={credentialData.confirmPassword}
                onChange={(e) => {
                  setCredentialData({ ...credentialData, confirmPassword: e.target.value });
                  if (credentialErrors.confirmPassword) {
                    setCredentialErrors({ ...credentialErrors, confirmPassword: "" });
                  }
                }}
                error={Boolean(credentialErrors.confirmPassword)}
                helperText={credentialErrors.confirmPassword}
              />
            </Stack>
          </Stack>

          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button
              variant="contained"
              onClick={handleCredentialSubmit}
              disabled={isUpdatingProfile || !credentialData.newPassword}
            >
              {isUpdatingProfile ? "Updating..." : "Update Password"}
            </Button>
          </Stack>
        </Stack>
      )}
    </Stack>
  );
};
