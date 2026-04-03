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
import { useParams } from 'react-router-dom';
import {
  Form,
  FormControl,
  FormHelperText,
  MenuItem,
  Select,
  Typography,
  Stack,
  Button,
  CircularProgress,
} from '@wso2/oxygen-ui';
import { Plus as Add } from '@wso2/oxygen-ui-icons-react';
import { useListGitSecrets } from '@agent-management-platform/api-client';
import type { CreateAgentFormValues } from '../form/schema';
import { CreateGitSecretModal } from './CreateGitSecretModal';

interface GitSecretSelectorProps {
  formData: CreateAgentFormValues;
  handleFieldChange: (field: keyof CreateAgentFormValues, value: unknown) => void;
  errors: Record<string, string | undefined>;
}

export const GitSecretSelector = ({
  formData,
  handleFieldChange,
  errors,
}: GitSecretSelectorProps) => {
  const { orgId } = useParams<{ orgId: string }>();
  const [isModalOpen, setIsModalOpen] = useState(false);

  const {
    data: gitSecretsData,
    isLoading,
    refetch,
  } = useListGitSecrets({ orgName: orgId }, { limit: 100 });

  const secrets = gitSecretsData?.secrets || [];

  const handleSecretChange = useCallback(
    (event: { target: { value: string } }) => {
      const value = event.target.value;
      if (value === '__create_new__') {
        setIsModalOpen(true);
        return;
      }
      handleFieldChange('gitSecretRef', value || undefined);
    },
    [handleFieldChange]
  );

  const handleSecretCreated = useCallback(
    (secretName: string) => {
      handleFieldChange('gitSecretRef', secretName);
      refetch();
      setIsModalOpen(false);
    },
    [handleFieldChange, refetch]
  );

  return (
    <>
      <Form.ElementWrapper label="Git Secret (Optional)" name="gitSecretRef">
        <FormControl fullWidth error={!!errors.gitSecretRef}>
          <Select
            id="gitSecretRef"
            value={formData.gitSecretRef || ''}
            onChange={handleSecretChange}
            displayEmpty
            disabled={isLoading}
            startAdornment={
              isLoading ? (
                <CircularProgress size={20} sx={{ mr: 1 }} />
              ) : undefined
            }
          >
            <MenuItem value="">
              <em>Public Repo (None)</em>
            </MenuItem>
            {secrets.map((secret) => (
              <MenuItem key={secret.name} value={secret.name}>
                <Typography>{secret.name}</Typography>
              </MenuItem>
            ))}
            <MenuItem value="__create_new__">
              <Stack direction="row" spacing={1} alignItems="center">
                <Button
                  variant="text"
                  size="small"
                  startIcon={<Add size={16} />}
                  sx={{ textTransform: 'none', p: 0 }}
                >
                  Create new git secret
                </Button>
              </Stack>
            </MenuItem>
          </Select>
          {errors.gitSecretRef ? (
            <FormHelperText error>{errors.gitSecretRef}</FormHelperText>
          ) : (
            <FormHelperText>
              Select a git secret for private repository authentication
            </FormHelperText>
          )}
        </FormControl>
      </Form.ElementWrapper>

      <CreateGitSecretModal
        open={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSecretCreated={handleSecretCreated}
      />
    </>
  );
};
