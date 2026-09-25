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

import React, { useCallback, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Form,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Check, Copy, Link2 } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useInviteUser } from "@agent-management-platform/api-client";
import {
  useFormValidation,
  useDirtyState,
} from "@agent-management-platform/views";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import { inviteUserSchema, type InviteUserFormValues } from "./forms/schemas";

export const UserInvitePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [formData, setFormData] = useState<InviteUserFormValues>({ email: "" });
  const [inviteLink, setInviteLink] = useState<string | undefined>();
  const [copied, setCopied] = useState(false);

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<InviteUserFormValues>(inviteUserSchema);
  const { checkDirty, resetDirty } = useDirtyState(formData);
  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const {
    mutateAsync: inviteUser,
    isPending: isInviting,
    error: inviteError,
  } = useInviteUser();

  const usersPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.users.path,
        { orgId },
      )
    : "#";

  const handleFieldChange = useCallback(
    (field: keyof InviteUserFormValues, value: string) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      checkDirty(newData);
      setFieldError(field, validateField(field, value));
    },
    [formData, checkDirty, validateField, setFieldError],
  );

  const handleSubmit = useCallback(async () => {
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    }
    setLastSubmittedValidationErrors({});

    try {
      const result = await inviteUser({
        params: { orgName: orgId },
        body: { email: formData.email.trim() },
      });
      setInviteLink(result.inviteLink);
    } catch {
      // inviteError state is set by React Query and displayed in the Alert above
    }
  }, [formData, validateForm, errors, inviteUser, orgId]);

  const handleInviteAnother = useCallback(() => {
    setInviteLink(undefined);
    setFormData({ email: "" });
    resetDirty();
    clearErrors();
    setLastSubmittedValidationErrors({});
  }, [resetDirty, clearErrors]);

  const handleCopy = useCallback(async () => {
    if (!inviteLink) return;
    await navigator.clipboard.writeText(inviteLink);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [inviteLink]);

  const submitErrors = Object.values(lastSubmittedValidationErrors);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {inviteError != null && (
          <Alert severity="error">
            {(inviteError as Error)?.message ?? "Failed to send invitation"}
          </Alert>
        )}

        {inviteLink == null ? (
          <>
            <Form.Stack spacing={3}>
              <Form.Section>
                <Form.Subheader>Invite Details</Form.Subheader>
                <Form.Stack spacing={2}>
                  <Form.ElementWrapper label="Email Address" name="email">
                    <TextField
                      slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.SHORT_TEXT } }}
                      id="email"
                      type="email"
                      value={formData.email}
                      onChange={(e) =>
                        handleFieldChange("email", e.target.value)
                      }
                      placeholder="user@example.com"
                      autoComplete="off"
                      error={!!errors.email}
                      helperText={errors.email}
                      fullWidth
                    />
                  </Form.ElementWrapper>
                </Form.Stack>
              </Form.Section>
            </Form.Stack>

            <Box display="flex" flexDirection="column" gap={3}>
              <Collapse
                in={submitErrors.length > 0}
                timeout="auto"
                unmountOnExit
              >
                <Alert severity="error">
                  {submitErrors.map((error, index) => (
                    <Box key={index}>{error}</Box>
                  ))}
                </Alert>
              </Collapse>
              <Box
                display="flex"
                flexDirection="row"
                gap={1}
                alignItems="center"
              >
                <Button
                  variant="outlined"
                  color="primary"
                  onClick={() => navigate(usersPath)}
                  disabled={isInviting}
                >
                  Cancel
                </Button>
                <Button
                  variant="contained"
                  color="primary"
                  startIcon={<Link2 size={16} />}
                  onClick={handleSubmit}
                  disabled={isInviting || !formData.email.trim()}
                >
                  Get Invitation Link
                </Button>
              </Box>
            </Box>
          </>
        ) : (
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Subheader>Invitation Created</Form.Subheader>
              <Form.Stack spacing={2}>
                <Alert severity="success">
                  An invitation has been created for{" "}
                  <strong>{formData.email}</strong>. Share the link below with
                  the user to complete registration.
                </Alert>

                <Box>
                  <Typography variant="body2" color="text.secondary" mb={1}>
                    Invite Link
                  </Typography>
                  <Box
                    display="flex"
                    flexDirection="row"
                    gap={1}
                    alignItems="center"
                  >
                    <TextField
                      fullWidth
                      value={inviteLink}
                      InputProps={{ readOnly: true }}
                    />
                    <Tooltip title={copied ? "Copied!" : "Copy link"}>
                      <IconButton onClick={handleCopy} size="small">
                        {copied ? <Check size={16} /> : <Copy size={16} />}
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Box>

                <Box
                  display="flex"
                  flexDirection="row"
                  gap={1}
                  alignItems="center"
                  justifyContent="flex-end"
                >
                  <Button
                    variant="outlined"
                    color="primary"
                    onClick={handleInviteAnother}
                  >
                    Invite Another
                  </Button>
                  <Button
                    variant="contained"
                    color="primary"
                    onClick={() => navigate(usersPath)}
                  >
                    Done
                  </Button>
                </Box>
              </Form.Stack>
            </Form.Section>
          </Form.Stack>
        )}
      </Box>
    </>
  );
};
