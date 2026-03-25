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

import { useState, useCallback } from 'react';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Form,
  TextField,
  Typography,
  Alert,
} from '@wso2/oxygen-ui';
import { useCreateGitSecret } from '@agent-management-platform/api-client';

interface CreateGitSecretModalProps {
  open: boolean;
  onClose: () => void;
  onSecretCreated: (secretName: string) => void;
  orgId: string;
}

interface FormState {
  name: string;
  username: string;
  password: string;
}

const initialFormState: FormState = {
  name: '',
  username: '',
  password: '',
};

export const CreateGitSecretModal = ({
  open,
  onClose,
  onSecretCreated,
  orgId,
}: CreateGitSecretModalProps) => {
  const [formState, setFormState] = useState<FormState>(initialFormState);
  const [errors, setErrors] = useState<Partial<Record<keyof FormState, string>>>({});

  const { mutate: createSecret, isPending, error } = useCreateGitSecret();

  const handleFieldChange = useCallback(
    (field: keyof FormState, value: string) => {
      setFormState((prev) => ({ ...prev, [field]: value }));
      // Clear error when user starts typing
      if (errors[field]) {
        setErrors((prev) => ({ ...prev, [field]: undefined }));
      }
    },
    [errors]
  );

  const validateForm = useCallback((): boolean => {
    const newErrors: Partial<Record<keyof FormState, string>> = {};

    if (!formState.name.trim()) {
      newErrors.name = 'Name is required';
    } else if (!/^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]$/.test(formState.name) && formState.name.length > 1) {
      newErrors.name = 'Name must start and end with alphanumeric characters';
    } else if (formState.name.length < 2) {
      newErrors.name = 'Name must be at least 2 characters';
    } else if (formState.name.length > 63) {
      newErrors.name = 'Name must be at most 63 characters';
    }

    if (!formState.username.trim()) {
      newErrors.username = 'Username is required';
    }
    if (!formState.password.trim()) {
      newErrors.password = 'Password/PAT is required';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  }, [formState]);

  const handleSubmit = useCallback(() => {
    if (!validateForm()) {
      return;
    }

    createSecret(
      {
        params: { orgName: orgId },
        body: {
          name: formState.name.trim(),
          type: 'basic-auth',
          credentials: {
            username: formState.username.trim(),
            password: formState.password,
          },
        },
      },
      {
        onSuccess: () => {
          onSecretCreated(formState.name.trim());
          setFormState(initialFormState);
        },
      }
    );
  }, [formState, validateForm, createSecret, orgId, onSecretCreated]);

  const handleClose = useCallback(() => {
    setFormState(initialFormState);
    setErrors({});
    onClose();
  }, [onClose]);

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create Git Secret</DialogTitle>
      <DialogContent>
        <Form.Stack spacing={3} sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Create a secret to authenticate with private Git repositories. This secret
            will be available for all projects in your organization.
          </Typography>

          {!!error && (
            <Alert severity="error">
              Failed to create git secret. Please try again.
            </Alert>
          )}

          <Form.ElementWrapper label="Secret Name" name="name">
            <TextField
              id="name"
              placeholder="e.g., my-github-pat"
              value={formState.name}
              onChange={(e) => handleFieldChange('name', e.target.value)}
              error={!!errors.name}
              helperText={errors.name || 'A unique name for this secret'}
              fullWidth
            />
          </Form.ElementWrapper>

          <Form.ElementWrapper label="Username" name="username">
            <TextField
              id="username"
              placeholder="e.g., your-github-username"
              value={formState.username}
              onChange={(e) => handleFieldChange('username', e.target.value)}
              error={!!errors.username}
              helperText={errors.username || 'Your Git username'}
              fullWidth
            />
          </Form.ElementWrapper>

          <Form.ElementWrapper label="Personal Access Token" name="password">
            <TextField
              id="password"
              placeholder="ghp_xxxxxxxxxxxx"
              type="password"
              value={formState.password}
              onChange={(e) => handleFieldChange('password', e.target.value)}
              error={!!errors.password}
              helperText={
                errors.password ||
                'Your personal access token (PAT) with repo access'
              }
              fullWidth
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={isPending}
        >
          {isPending ? 'Creating...' : 'Create Secret'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
